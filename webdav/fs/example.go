package fs

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/rfielding/webdev/webdav"
)

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
  Create a webdav handler.
*/
func buildHandler(dir string) {
	// wire together a handler
	fs := FS{Root: dir}
	allowed := func(ctx context.Context, name string, allow webdav.Allow) bool {
		// not bothering to check the values at the moment
		username, _ := ctx.Value("username").(string)
		if _, err := os.Stat(name); os.IsNotExist(err) {
			return false
		}
		log.Printf("WEBDAV %s allowed %s on %s", username, allow, name)
		return true
	}
	fs.AllowHandler = allowed

	// The raw webdav handler that doesn't have a context set
	srv := &webdav.Handler{
		FileSystem: fs,
		LockSystem: NewMemLS(),
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

		go http.ListenAndServeTLS(fmt.Sprintf(":%d", port), "cert.pem", "key.pem", nil)
	}
	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
		log.Fatalf("Error with WebDAV server: %v", err)
	}
}
