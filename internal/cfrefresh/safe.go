package cfrefresh

import "github.com/crmmc/grokpi/internal/logging"

func safeGo(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logging.Error("goroutine panic recovered", "name", name, "panic", r)
			}
		}()
		fn()
	}()
}
