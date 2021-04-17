package monotime

import "time"

// Now returns the current time more precisely for Web and Windows targets
func Now() time.Duration {
	return now()
}
