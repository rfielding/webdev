fs
========

This is a simple example implementation of the FileSystem that is complex enough to be used in a real case.  It implements a simple permission system by way of using Open Policy Agent as a language for performing the calculation of permissions.

- Before uploading a file, write an Open Policy Agent file either named after the file, or some directory above it.
- The calculated permiissions will apply to this metadata file as well.
- For an upload of `/rob/cat.jpg`, first write a file `/rob/.__cat.jpg.rego` to specifically security-label the file.
- To apply more general security labels, you can just place it on the directory instead `/rob/.__thisdir.rego`


```rego
Stat = true                   # everyone can see the file in listings
Read{true}                    # everyone can http GET the file

Write{
  input.claims.groups.username[_] == "rob"
}                             # only username rob can edit the file
Delete{Write}                 # can delete the file

Banner = "PRIVATE"            # if you need a banner to label the file, use this
BannerForeground = "white"    # rendering hints pen color of banner
BannerBackground = "red"      # rendering hints text background
```	

This policy has to calculate on an input.  The input we expect are the JWT claims of a token.  It is TBD to get the claims actually pulled in as a result of logging in.  Here is an example of a JWT token claims extracted, with fields like expiration and issuer not included:

> /rob/.__claims.json
```json
{
	"exp": 80943290828390,
	"iss": "rfielding.io",
	"groups": {
		"username": ["rob"],
		"age": ["adult"],
		"citizen": ["US"]
	}
}

```

The JWT claims get plugged into the `input.claims` during evaluation of the rego policy.

The top-level directory is a special directory.  If your username matches, then you should be allowed to MKCOL on the home directory to create your own user.  This is user self-service, so that there is no system administrator to get users started with a space that they are allowed to write into.
