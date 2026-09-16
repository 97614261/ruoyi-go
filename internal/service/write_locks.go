package service

import "sync"

// The deployment contract is one Go process. These domain locks make each
// uniqueness check and its following write one in-process critical section.
// If the service is ever deployed with multiple processes, replace this with
// a cross-process lock or database uniqueness constraints.
var (
	userWriteMu         sync.Mutex
	roleWriteMu         sync.Mutex
	postWriteMu         sync.Mutex
	configWriteMu       sync.Mutex
	dictWriteMu         sync.Mutex
	deptWriteMu         sync.Mutex
	menuWriteMu         sync.Mutex
	jobWriteMu          sync.Mutex
	genWriteMu          sync.RWMutex
	permissionRefreshMu sync.Mutex
)
