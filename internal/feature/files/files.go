// Package files owns the one process-wide user asset store.
// Consumers discover/select with Get().List() and stream with Get().Open(name).
package files

import (
	"github.com/ygelfand/echolocal/internal/layout"
	"github.com/ygelfand/echolocal/internal/userfiles"
	"sync"
)

var get = sync.OnceValue(func() *userfiles.Store {
	return userfiles.New(layout.StateDir + "/user-files")
})

func Get() *userfiles.Store { return get() }
