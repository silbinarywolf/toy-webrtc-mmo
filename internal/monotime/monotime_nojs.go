// +build !js,!windows

package monotime

import (
	"time"
)

func now() time.Duration {
	return time.Now().UnixNano()
}
