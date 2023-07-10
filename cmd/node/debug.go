package main

import (
	"net/http"

	"github.com/ipfs/kubo/profile"
)

func init() {
	//////////////////// Falcon ////////////////////
	if true {
		return
	}
	//////////////////// Falcon ////////////////////

	http.HandleFunc("/debug/stack",
		func(w http.ResponseWriter, _ *http.Request) {
			_ = profile.WriteAllGoroutineStacks(w)
		},
	)
}
