// +build js

package monotime

import (
	"time"

	"syscall/js"
)

func now() time.Duration {
	// time.Now() is not reliable until GopherJS supports performance.now().
	return time.Duration(js.Global().Get("performance").Call("now").Float() * float64(time.Millisecond))
}
