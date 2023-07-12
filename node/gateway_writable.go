package node

import (
	"context"
	"fmt"
	"net/http"
	"os"
	gopath "path"

	coreiface "github.com/ipfs/boxo/coreiface"
	"github.com/ipfs/boxo/files"

	ipfsgw "github.com/ipfs/boxo/gateway"
	dag "github.com/ipfs/boxo/ipld/merkledag"
	"github.com/ipfs/boxo/mfs"
	"github.com/ipfs/boxo/path"
	"github.com/ipfs/boxo/path/resolver"
	cid "github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	routing "github.com/libp2p/go-libp2p/core/routing"

	"github.com/photon-storage/go-common/log"
)

const (
	ipfsPathPrefix = "/ipfs/"
)

// ***********************************************************************//
// gatewayAPI is copied from github.com/ipfs/kubo/core/corehttp/gateway_writable.go
// with code format and log change.
// ***********************************************************************//
type writableGatewayHandler struct {
	api    coreiface.CoreAPI
	config *ipfsgw.Config
}

func (h *writableGatewayHandler) handlePost(
	w http.ResponseWriter,
	r *http.Request,
) {
	p, err := h.api.Unixfs().Add(r.Context(), files.NewReaderFile(r.Body))
	if err != nil {
		internalWebError(w, err)
		return
	}

	h.addUserHeaders(w) // ok, _now_ write user's headers.
	w.Header().Set("IPFS-Hash", p.Cid().String())
	log.Debug("CID created, http redirect",
		"from", r.URL,
		"to", p,
		"status", http.StatusCreated,
	)
	http.Redirect(w, r, p.String(), http.StatusCreated)
}

func (h *writableGatewayHandler) handlePut(
	w http.ResponseWriter,
	r *http.Request,
) {
	ctx := r.Context()
	ds := h.api.Dag()

	// Parse the path
	rootCid, newPath, err := parseIpfsPath(r.URL.Path)
	if err != nil {
		webError(
			w,
			"WritableGateway: failed to parse the path",
			err,
			http.StatusBadRequest,
		)
		return
	}
	if newPath == "" || newPath == "/" {
		http.Error(w, "WritableGateway: empty path", http.StatusBadRequest)
		return
	}
	newDirectory, newFileName := gopath.Split(newPath)

	// Resolve the old root.

	rnode, err := ds.Get(ctx, rootCid)
	if err != nil {
		webError(
			w,
			"WritableGateway: Could not create DAG from request",
			err,
			http.StatusInternalServerError,
		)
		return
	}

	pbnd, ok := rnode.(*dag.ProtoNode)
	if !ok {
		webError(
			w,
			"Cannot read non protobuf nodes through gateway",
			dag.ErrNotProtobuf,
			http.StatusBadRequest,
		)
		return
	}

	// Create the new file.
	newFilePath, err := h.api.Unixfs().Add(ctx, files.NewReaderFile(r.Body))
	if err != nil {
		webError(
			w,
			"WritableGateway: could not create DAG from request",
			err,
			http.StatusInternalServerError,
		)
		return
	}

	newFile, err := ds.Get(ctx, newFilePath.Cid())
	if err != nil {
		webError(
			w,
			"WritableGateway: failed to resolve new file",
			err,
			http.StatusInternalServerError,
		)
		return
	}

	// Patch the new file into the old root.

	root, err := mfs.NewRoot(ctx, ds, pbnd, nil)
	if err != nil {
		webError(
			w,
			"WritableGateway: failed to create MFS root",
			err,
			http.StatusBadRequest,
		)
		return
	}

	if newDirectory != "" {
		if err := mfs.Mkdir(
			root,
			newDirectory,
			mfs.MkdirOpts{
				Mkparents: true,
				Flush:     false,
			},
		); err != nil {
			webError(
				w,
				"WritableGateway: failed to create MFS directory",
				err,
				http.StatusInternalServerError,
			)
			return
		}
	}

	dirNode, err := mfs.Lookup(root, newDirectory)
	if err != nil {
		webError(
			w,
			"WritableGateway: failed to lookup directory",
			err,
			http.StatusInternalServerError,
		)
		return
	}

	dir, ok := dirNode.(*mfs.Directory)
	if !ok {
		http.Error(
			w,
			"WritableGateway: target directory is not a directory",
			http.StatusBadRequest,
		)
		return
	}

	err = dir.Unlink(newFileName)
	switch err {
	case os.ErrNotExist, nil:
	default:
		webError(
			w,
			"WritableGateway: failed to replace existing file",
			err,
			http.StatusBadRequest,
		)
		return
	}

	if err = dir.AddChild(newFileName, newFile); err != nil {
		webError(
			w,
			"WritableGateway: failed to link file into directory",
			err,
			http.StatusInternalServerError,
		)
		return
	}

	nnode, err := root.GetDirectory().GetNode()
	if err != nil {
		webError(
			w,
			"WritableGateway: failed to finalize",
			err,
			http.StatusInternalServerError,
		)
		return
	}
	newcid := nnode.Cid()

	h.addUserHeaders(w) // ok, _now_ write user's headers.
	w.Header().Set("IPFS-Hash", newcid.String())

	redirectURL := gopath.Join(ipfsPathPrefix, newcid.String(), newPath)
	log.Debug("CID replaced, redirect",
		"from", r.URL,
		"to", redirectURL,
		"status", http.StatusCreated,
	)
	http.Redirect(w, r, redirectURL, http.StatusCreated)
}

func (h *writableGatewayHandler) handleDelete(
	w http.ResponseWriter,
	r *http.Request,
) {
	ctx := r.Context()

	// parse the path

	rootCid, newPath, err := parseIpfsPath(r.URL.Path)
	if err != nil {
		webError(
			w,
			"WritableGateway: failed to parse the path",
			err,
			http.StatusBadRequest,
		)
		return
	}
	if newPath == "" || newPath == "/" {
		http.Error(
			w,
			"WritableGateway: empty path",
			http.StatusBadRequest,
		)
		return
	}
	directory, filename := gopath.Split(newPath)

	// lookup the root

	rootNodeIPLD, err := h.api.Dag().Get(ctx, rootCid)
	if err != nil {
		webError(
			w,
			"WritableGateway: failed to resolve root CID",
			err,
			http.StatusInternalServerError,
		)
		return
	}

	rootNode, ok := rootNodeIPLD.(*dag.ProtoNode)
	if !ok {
		http.Error(
			w,
			"WritableGateway: empty path",
			http.StatusInternalServerError,
		)
		return
	}

	// construct the mfs root

	root, err := mfs.NewRoot(ctx, h.api.Dag(), rootNode, nil)
	if err != nil {
		webError(
			w,
			"WritableGateway: failed to construct the MFS root",
			err,
			http.StatusBadRequest,
		)
		return
	}

	// lookup the parent directory

	parentNode, err := mfs.Lookup(root, directory)
	if err != nil {
		webError(
			w,
			"WritableGateway: failed to look up parent",
			err,
			http.StatusInternalServerError,
		)
		return
	}

	parent, ok := parentNode.(*mfs.Directory)
	if !ok {
		http.Error(
			w,
			"WritableGateway: parent is not a directory",
			http.StatusInternalServerError,
		)
		return
	}

	// delete the file

	switch parent.Unlink(filename) {
	case nil, os.ErrNotExist:
	default:
		webError(
			w,
			"WritableGateway: failed to remove file",
			err,
			http.StatusInternalServerError,
		)
		return
	}

	nnode, err := root.GetDirectory().GetNode()
	if err != nil {
		webError(
			w,
			"WritableGateway: failed to finalize",
			err,
			http.StatusInternalServerError,
		)
		return
	}
	ncid := nnode.Cid()

	h.addUserHeaders(w) // ok, _now_ write user's headers.
	w.Header().Set("IPFS-Hash", ncid.String())

	redirectURL := gopath.Join(ipfsPathPrefix+ncid.String(), directory)
	// note: StatusCreated is technically correct here as we created a new resource.
	log.Debug("CID deleted, redirect",
		"from", r.RequestURI,
		"to", redirectURL,
		"status", http.StatusCreated,
	)
	http.Redirect(w, r, redirectURL, http.StatusCreated)
}

func (h *writableGatewayHandler) addUserHeaders(w http.ResponseWriter) {
	for k, v := range h.config.Headers {
		w.Header()[http.CanonicalHeaderKey(k)] = v
	}
}

func parseIpfsPath(p string) (cid.Cid, string, error) {
	rootPath, err := path.ParsePath(p)
	if err != nil {
		return cid.Cid{}, "", err
	}

	// Check the path.
	rsegs := rootPath.Segments()
	if rsegs[0] != "ipfs" {
		return cid.Cid{}, "", fmt.Errorf("WritableGateway: only ipfs paths supported")
	}

	rootCid, err := cid.Decode(rsegs[1])
	if err != nil {
		return cid.Cid{}, "", err
	}

	return rootCid, path.Join(rsegs[2:]), nil
}

func webError(
	w http.ResponseWriter,
	message string,
	err error,
	defaultCode int,
) {
	if _, ok := err.(resolver.ErrNoLink); ok {
		webErrorWithCode(w, message, err, http.StatusNotFound)
	} else if err == routing.ErrNotFound {
		webErrorWithCode(w, message, err, http.StatusNotFound)
	} else if ipld.IsNotFound(err) {
		webErrorWithCode(w, message, err, http.StatusNotFound)
	} else if err == context.DeadlineExceeded {
		webErrorWithCode(w, message, err, http.StatusRequestTimeout)
	} else {
		webErrorWithCode(w, message, err, defaultCode)
	}
}

func webErrorWithCode(
	w http.ResponseWriter,
	message string,
	err error,
	code int,
) {
	http.Error(w, fmt.Sprintf("%s: %s", message, err), code)
	if code >= 500 {
		log.Warn("Server error", "message", message, "error", err)
	}
}

// return a 500 error and log
func internalWebError(w http.ResponseWriter, err error) {
	webErrorWithCode(w, "internalWebError", err, http.StatusInternalServerError)
}
