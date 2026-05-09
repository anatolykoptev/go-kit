package httpmw

import "net/http"

// WalkAndRegister registers routes from a router that supports a Walk-style
// callback. The caller wires this to the router's Walk method:
//
//	err := httpmw.WalkAndRegister(func(register func(method, pattern string, h http.Handler)) error {
//	    return chi.Walk(router, func(method, route string, h http.Handler, _ ...func(http.Handler) http.Handler) error {
//	        register(method, route, h)
//	        return nil
//	    })
//	})
//
// Call after all routes are mounted so the registry captures every route.
// WalkAndRegister does not import chi — it accepts any router via the
// structural walk function signature.
func WalkAndRegister(walk func(register func(method, pattern string, h http.Handler)) error) error {
	return walk(func(method, pattern string, h http.Handler) {
		registerHandler(method, pattern, h)
	})
}
