package main

import (
	"sync"
)

type Releases struct {
	// repo => version => filename => download url
	data map[string]map[string]map[string]string
	lock sync.Mutex
}

func (r *Releases) Exist(repo, version, filename string) bool {
	r.lock.Lock()
	defer r.lock.Unlock()

	if _, ok := r.data[repo]; !ok {
		return false
	}
	if _, ok := r.data[repo][version]; !ok {
		return false
	}
	if _, ok := r.data[repo][version][filename]; !ok {
		return false
	}
	return true
}

func (r *Releases) Add(repo, version, filename, url string) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if _, ok := r.data[repo]; !ok {
		r.data[repo] = make(map[string]map[string]string)
	}
	if _, ok := r.data[repo][version]; !ok {
		r.data[repo][version] = make(map[string]string)
	}
	r.data[repo][version][filename] = url
}
