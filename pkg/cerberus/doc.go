// Package cerberus provides authentication, authorization, and audit capabilities
// for the Tartarus platform.
//
// Cerberus is the three-headed guardian of the Underworld, and this package
// implements the three heads of access control:
//   - Head 1 (Authentication): Verifies identity
//   - Head 2 (Authorization): Checks permissions
//   - Head 3 (Audit): Records all access attempts
//
// # Basic Usage
//
// Create a simple API key authenticator:
//
//	auth := cerberus.NewSimpleAPIKeyAuthenticator("your-secret-key")
//	authz := cerberus.NewAllowAllAuthorizer()
//	audit := cerberus.NewLogAuditor(logger)
//	gateway := cerberus.NewGateway(auth, authz, audit)
//
// Wrap your HTTP handlers:
//
//	middleware := cerberus.NewHTTPMiddleware(
//	    gateway,
//	    cerberus.NewBearerTokenExtractor(),
//	    cerberus.NewDefaultResourceMapper(),
//	)
//	handler := middleware.Wrap(yourHandler)
//
// # Advanced Usage with RBAC
//
// Define role-based access control policies:
//
//	policies := map[string]*cerberus.RBACPolicy{
//	    "admin": {
//	        Role: "admin",
//	        Permissions: []cerberus.Permission{
//	            {AllowAll: true},
//	        },
//	    },
//	    "user": {
//	        Role: "user",
//	        Permissions: []cerberus.Permission{
//	            {
//	                Actions:   []cerberus.Action{cerberus.ActionCreate, cerberus.ActionRead},
//	                Resources: []cerberus.ResourceType{cerberus.ResourceTypeSandbox},
//	            },
//	        },
//	    },
//	}
//	authz := cerberus.NewRBACAuthorizer(policies)
//
// # Retrieving Identity
//
// Get the authenticated identity in your handlers:
//
//	func myHandler(w http.ResponseWriter, r *http.Request) {
//	    identity, ok := cerberus.GetIdentity(r.Context())
//	    if !ok {
//	        http.Error(w, "Unauthorized", http.StatusUnauthorized)
//	        return
//	    }
//	    // Use identity.ID, identity.Roles, etc.
//	}
package cerberus
