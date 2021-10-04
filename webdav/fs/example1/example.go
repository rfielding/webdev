package example1

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/open-policy-agent/opa/rego"
	"github.com/rfielding/webdev/webdav"
	"github.com/rfielding/webdev/webdav/fs"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
)

/*
  Simple json dump utility
*/
func AsJson(obj interface{}) string {
	j, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		log.Printf("cannot convert AsJson: %v", err)
	}
	return string(j)
}

/*
  Calculate some permissions
*/
func evalRego(claims interface{}, opaObj string) (map[string]interface{}, error) {
	ctx := context.TODO()

	compiler := rego.New(
		rego.Query("data.policy"),
		rego.Module("policy.rego", opaObj),
	)

	query, err := compiler.PrepareForEval(ctx)

	if err != nil {
		return nil, err
	}

	results, err := query.Eval(ctx, rego.EvalInput(claims))
	if err != nil {
		return nil, fmt.Errorf("while evaulating opaObj: %s: %v", opaObj, err)
	}
	return results[0].Expressions[0].Value.(map[string]interface{}), nil
}

func ExampleMain() {

	// parse environmental setup
	dirFlag := flag.String("d", "./data", "Directory to serve from. Default is CWD")
	httpPort := flag.Int("p", 8000, "Port to serve on (Plain HTTP)")
	serveSecure := flag.Bool("s", false, "Serve HTTPS. Default false")
	flag.Parse()

	buildHandler(*dirFlag)
	listenTo(*httpPort, *serveSecure == true)
}

/*
 This just ensures that the handler is wrapped up
 in a context that has the username and password,
 so that the filesystem can have some context.
*/
type authWrappedHandler struct {
	Handler http.Handler
}

/**
Wrap in trivial authentication so that the permission system can work.
*/
func (a *authWrappedHandler) ServeHTTP(
	w http.ResponseWriter,
	r *http.Request,
) {
	w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
	username, password, ok := r.BasicAuth()
	if !ok {
		// come back with a username and password
		http.Error(w, "Not authorized", 401)
		return
	}
	ctx := r.Context()
	ctx = context.WithValue(ctx, "username", username)
	ctx = context.WithValue(ctx, "password", password)
	r = r.WithContext(ctx)
	a.Handler.ServeHTTP(w, r)
}

/*
  If we were to serialize permissions, these are the known
  fields.
*/
type Permission struct {
	Create           bool   `json:"Create,omitempty"`
	Read             bool   `json:"Read,omitempty"`
	Write            bool   `json:"Write,omitempty"`
	Delete           bool   `json:"Delete,omitempty"`
	Stat             bool   `json:"Stat,omitempty"`
	Banner           string `json:"Banner,omitempty`
	BannerForeground string `json:"BannerForeground,omitempty`
	BannerBackground string `json:"BannerBackground,omitempty`
}

/*
  This is effectively a set of LDAP groups to model a user,
  as just a set of multi-valued attributes.
*/
type Claims struct {
	Groups map[string][]string `json:"groups"`
}

type ClaimsContext struct {
	Claims Claims
	Action fs.Action
}

/*
  Return this when something went wrong.
*/
var emptyClaims = ClaimsContext{
	Claims: Claims{Groups: make(map[string][]string)},
	Action: fs.Action{},
}

/*
  Find the JWT claims for the currently logged in user,
  and also inject context of what we are trying to do,
  as that may be part of the calculation.
*/
func claimsInContext(root, username string, action fs.Action) interface{} {
	claimsFile := fmt.Sprintf("%s/%s/.__claims.json", root, username)
	if _, err := os.Stat(path.Dir(claimsFile)); os.IsNotExist(err) {
		err = os.Mkdir(path.Dir(claimsFile), 0744)
		if err != nil {
			log.Printf("WEBDAV: could not make home dir %s %v", path.Dir(claimsFile), err)
			return emptyClaims
		}
	}
	//log.Printf("use claims file %s", claimsFile)
	data, err := ioutil.ReadFile(claimsFile)
	if err != nil {
		log.Printf("WEBDAV: reading claims %v", err)
		return emptyClaims
	}
	var claims Claims
	err = json.Unmarshal(data, &claims)
	if err != nil {
		log.Printf("WEBDAV: unmarshal claims %v", err)
		return emptyClaims
	}
	return ClaimsContext{
		Claims: claims,
		Action: action,
	}
}

/*
  Calculate a bland policy with no privilege
  when something goes wrong with parsing policy.
*/
const emptyPolicy = `package policy
Create = false
Read = false
Write = false
Delete = false
Stat = false
Banner = "error"
BannerForeground = "white"
BannerBackground = "black"
`

/*
  Find the rego that applies to this file.
  Perhaps not for this file specifically,
  but via its parent.
*/
func regoOf(root, name string) string {
	regoFile := fs.NameFor(name, "security.rego")
	d := path.Dir(name)
	data, err := ioutil.ReadFile(regoFile)
	if d != "." && d != root && os.IsNotExist(err) {
		return regoOf(root, d)
	}
	if err != nil {
		log.Printf("WEBDAV: reading rego %v", err)
		return emptyPolicy
	}
	return string(data)
}

/*
  Create a webdav handler.
*/
func buildHandler(dir string) {
	// wire together a handler
	locks := fs.NewMemLS()
	fsys := fs.FS{Root: dir, Locks: locks}
	allowed := func(ctx context.Context, action fs.Action) map[string]interface{} {
		// not bothering to check the values at the moment
		username, _ := ctx.Value("username").(string)
		//		log.Printf("WEBDAV %s allowed %s on %s", username, allow, name)
		permission, err := evalRego(claimsInContext(fsys.Root, username, action), regoOf(fsys.Root, action.Name))
		if err != nil {
			log.Printf("WEBDAV: error evaluating rego: %v", err)
			return make(map[string]interface{})
		}
		log.Printf("permission: %s: %v", action.Name, AsJson(permission))
		return permission
	}
	fsys.PermissionHandler = allowed

	// The raw webdav handler that doesn't have a context set
	srv := &webdav.Handler{
		FileSystem: fsys,
		LockSystem: locks,
		Logger: func(r *http.Request, err error) {
			if err != nil {
				log.Printf("WEBDAV %s [%s]: %s, ERROR: %s\n", r.Context().Value("username"), r.Method, r.URL, err)
			} else {
				log.Printf("WEBDAV %s [%s]: %s \n", r.Context().Value("username"), r.Method, r.URL)
			}
		},
	}

	// ok... handle http or https
	http.Handle("/", &authWrappedHandler{Handler: srv})
}

/*
  Generic listener setup.  Use a TLS cert with a SAN of localhost, to make things easier.
*/
func listenTo(port int, secure bool) {
	if secure {
		if _, err := os.Stat("./cert.pem"); err != nil {
			fmt.Println("[x] No cert.pem in current directory. Please provide a valid cert")
			return
		}
		if _, er := os.Stat("./key.pem"); er != nil {
			fmt.Println("[x] No key.pem in current directory. Please provide a valid cert")
			return
		}

		log.Printf("Starting server at https://0.0.0.0:%d", port)
		http.ListenAndServeTLS(fmt.Sprintf(":%d", port), "cert.pem", "key.pem", nil)
	}
	log.Printf("Starting server at http://127.0.0.1:%d", port)
	if err := http.ListenAndServe(fmt.Sprintf("127.0.0.1:%d", port), nil); err != nil {
		log.Fatalf("Error with WebDAV server: %v", err)
	}
}
