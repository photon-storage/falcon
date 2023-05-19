package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	format "github.com/ipfs/go-ipld-format"
	ipfsfiles "github.com/ipfs/go-libipfs/files"
	"github.com/ipfs/go-merkledag"
	"github.com/ipfs/go-mfs"
	dagcmd "github.com/ipfs/kubo/core/commands/dag"
	"github.com/ipfs/kubo/core/commands/name"
	"github.com/ipfs/kubo/core/commands/pin"

	"github.com/photon-storage/falcon/node/consts"
)

var checks = []*check{
	// "GET /ipfs/<CID>" retrieves content of CID in the format specified
	// by the "Accept" header.
	&check{
		desc: "IPFS: GET root object by CID",
		run: func(ctx context.Context, cfg config) error {
			k := "QmSnuWmxptJZdLJpKRarxBMS2Ju2oANVrgbr2xWbie9b2D"
			for _, t := range []consts.AcceptMediaType{
				// consts.AcceptJson,
				// consts.AcceptCbor,
				// consts.AcceptXTar, --hanging
				consts.AcceptVndIpldRaw,
				// consts.AcceptVndIpldCar, --hanging
				consts.AcceptVndIpldDagJson,
				consts.AcceptVndIpldDagCbor,
				// consts.AcceptVndIpfsIpnsRecord,
			} {
				logStep("Fetch Accept Type: %v\n", t)

				header := http.Header{}
				header.Set("Accept", string(t))

				code, header, data, err := gatewayGet(
					ctx,
					cfg,
					fmt.Sprintf("ipfs/%s", k),
					header,
				)
				if err := logResp(code, header, data, err); err != nil {
					return err
				}
			}

			return nil
		},
	},
	// "POST /ipfs" uploads body data and returns CID and retrieval URL
	// in the response header.
	&check{
		desc: "IPFS: POST data, redirect and fetch",
		run: func(ctx context.Context, cfg config) error {
			logStep("Post data\n")

			code, header, data, err := gatewayPost(
				ctx,
				cfg,
				"ipfs/",
				nil,
				strings.NewReader(fmt.Sprintf(
					"Photon Gateway POST Test - %v",
					time.Now().Format(time.RFC822Z),
				)),
			)
			if err := logResp(code, header, data, err); err != nil {
				return err
			}

			logStep("Fetch posted data from redirected URL\n")

			redirect := header.Get("Location")
			if redirect == "" {
				return fmt.Errorf("Redirect URL missing")
			}

			header = http.Header{}
			header.Set("Accept", string(consts.AcceptVndIpldRaw))
			code, header, data, err = gatewayGet(
				ctx,
				cfg,
				redirect,
				header,
			)
			return logResp(code, header, data, err)
		},
	},
	// "PUT /ipfs/<CID>/<path>" creates file or directory of <path> under
	// location specified by <CID>.
	// "GET /ipfs/<CID>/<path>" retrieves file or directory content of <path>
	// under location specified by <CID>
	// "DELETE /ipfs/<CID>/<path>" deletes file or directory of <path> under
	// location specified by <CID>
	&check{
		desc: "IPFS: DAG operation with PUT, GET and DELETE",
		run: func(ctx context.Context, cfg config) error {
			logStep("Put file 0\n")

			// CID of empty directory.
			root := "QmUNLLsPACCz1vLxQVkXqqLX5R1X345qqfHbsf67hvA3Nn"
			path := "falcon_test0.txt"
			code, header, data, err := gatewayPut(
				ctx,
				cfg,
				fmt.Sprintf("ipfs/%s/%s", root, path),
				nil,
				strings.NewReader(fmt.Sprintf(
					"Photon Gateway PUT Test - File0 - %v",
					time.Now().Format(time.RFC822Z),
				)),
			)
			if err := logResp(code, header, data, err); err != nil {
				return err
			}

			logStep("Put file 1\n")

			root = header.Get("Ipfs-Hash")
			if root == "" {
				return fmt.Errorf("Root CID missing")
			}
			path = "falcon_test1.txt"
			code, header, data, err = gatewayPut(
				ctx,
				cfg,
				fmt.Sprintf("ipfs/%s/%s", root, path),
				nil,
				strings.NewReader(fmt.Sprintf(
					"Photon Gateway PUT Test - File1 - %v",
					time.Now().Format(time.RFC822Z),
				)),
			)
			if err := logResp(code, header, data, err); err != nil {
				return err
			}

			logStep("Put file 2\n")

			root = header.Get("Ipfs-Hash")
			if root == "" {
				return fmt.Errorf("Root CID missing")
			}

			path = "a_dir/falcon_test2.txt"
			code, header, data, err = gatewayPut(
				ctx,
				cfg,
				fmt.Sprintf("ipfs/%s/%s", root, path),
				nil,
				strings.NewReader(fmt.Sprintf(
					"Photon Gateway PUT Test - File2 - %v",
					time.Now().Format(time.RFC822Z),
				)),
			)
			if err := logResp(code, header, data, err); err != nil {
				return err
			}

			logStep("Put file 3\n")

			root = header.Get("Ipfs-Hash")
			if root == "" {
				return fmt.Errorf("Root CID missing")
			}
			path = "a_dir/falcon_test3.txt"
			code, header, data, err = gatewayPut(
				ctx,
				cfg,
				fmt.Sprintf("ipfs/%s/%s", root, path),
				nil,
				strings.NewReader(fmt.Sprintf(
					"Photon Gateway PUT Test - File3 - %v",
					time.Now().Format(time.RFC822Z),
				)),
			)
			if err := logResp(code, header, data, err); err != nil {
				return err
			}

			root = header.Get("Ipfs-Hash")
			if root == "" {
				return fmt.Errorf("Root CID missing")
			}

			logStep("Get a_dir\n")

			header = http.Header{}
			header.Set("Accept", string(consts.AcceptVndIpldRaw))
			code, header, data, err = gatewayGet(
				ctx,
				cfg,
				fmt.Sprintf("ipfs/%s/%s", root, "a_dir"),
				header,
			)
			if err := logResp(code, header, data, err); err != nil {
				return err
			}

			logStep("Get file 3\n")

			code, header, data, err = gatewayGet(
				ctx,
				cfg,
				fmt.Sprintf("ipfs/%s/%s", root, path),
				header,
			)
			if err := logResp(code, header, data, err); err != nil {
				return err
			}

			logStep("Delete file 3\n")

			code, header, data, err = gatewayDel(
				ctx,
				cfg,
				fmt.Sprintf("ipfs/%s/%s", root, path),
				nil,
			)
			if err := logResp(code, header, data, err); err != nil {
				return err
			}

			// The following two requests demonstrate PUT on ipfs/ creates
			// DAG nodes instead of MFS files.
			logStep("API files/ls\n")
			code, header, data, err = gatewayPost(
				ctx,
				cfg,
				fmt.Sprintf("api/v0/files/ls?arg=/&U=true"),
				nil,
				nil,
			)
			if err := logResp(code, header, data, err); err != nil {
				return err
			}

			logStep("API dag/get\n")
			code, header, data, err = gatewayPost(
				ctx,
				cfg,
				fmt.Sprintf("api/v0/dag/get?arg=%s&U=true", root),
				nil,
				nil,
			)
			return logResp(code, header, data, err)
		},
	},
	// "GET /ipns/<dns_path>" retrieves content pointed to by <dns_path>,
	// whose domain name is resolved to CID.
	&check{
		skip: true,
		desc: "IPNS: GET by DNS name",
		run: func(ctx context.Context, cfg config) error {
			k := "docs.ipfs.tech/index.html"
			for _, t := range []consts.AcceptMediaType{
				// consts.AcceptJson,
				// consts.AcceptCbor,
				// consts.AcceptXTar, --hanging
				consts.AcceptVndIpldRaw,
				// consts.AcceptVndIpldCar, --hanging
				consts.AcceptVndIpldDagJson,
				consts.AcceptVndIpldDagCbor,
				// consts.AcceptVndIpfsIpnsRecord,
			} {
				logStep("Fetch Accept Type: %v\n", t)

				header := http.Header{}
				header.Set("Accept", string(t))
				code, header, data, err := gatewayGet(
					ctx,
					cfg,
					fmt.Sprintf("ipns/%s", k),
					header,
				)
				if len(data) > 200 {
					data = data[:200]
				}

				if err := logResp(code, header, data, err); err != nil {
					return err
				}
			}

			return nil
		},
	},
	// "GET /ipns/<libp2p_pk>" resolves to IPNS record
	&check{
		desc: "IPNS: GET by Libp2pKey",
		run: func(ctx context.Context, cfg config) error {
			logStep("Post data\n")

			code, header, data, err := gatewayPost(
				ctx,
				cfg,
				"ipfs/",
				nil,
				strings.NewReader(fmt.Sprintf(
					"Photon Gateway IPNS Get by Libp2pKey Test - %v",
					time.Now().Format(time.RFC822Z),
				)),
			)
			if err := logResp(code, header, data, err); err != nil {
				return err
			}

			k := header.Get("Ipfs-Hash")
			if k == "" {
				return fmt.Errorf("CID missing")
			}

			logStep("Publish IPNS record\n")

			// RPC API
			code, header, data, err = gatewayPost(
				ctx,
				cfg,
				fmt.Sprintf("api/v0/name/publish?arg=%s", k),
				header,
				nil,
			)
			if err := logResp(code, header, data, err); err != nil {
				return err
			}

			res := name.IpnsEntry{}
			if err := json.Unmarshal(data, &res); err != nil {
				return err
			}

			logStep("Fetch IPNS record\n")

			header = http.Header{}
			header.Set("Accept", string(consts.AcceptVndIpfsIpnsRecord))
			code, header, data, err = gatewayGet(
				ctx,
				cfg,
				fmt.Sprintf("ipns/%s", res.Name),
				header,
			)
			return logResp(code, header, data, err)
		},
	},
	// "POST /api/v0/pin/ls" lists pinned CIDs.
	&check{
		desc: "API: List pinned CIDs",
		run: func(ctx context.Context, cfg config) error {
			logStep("List pins\n")

			code, header, data, err := gatewayPost(
				ctx,
				cfg,
				"api/v0/pin/ls?stream=false",
				nil,
				nil,
			)
			if err := logResp(code, header, data, err); err != nil {
				return err
			}

			return nil
		},
	},
	// !!! This check removes all local pins. Enable with care!!!
	// "POST /api/v0/pin/ls" lists pinned CIDs and use rm to remove them.
	&check{
		skip: true,
		desc: "API: Clean up all pinned CIDs",
		run: func(ctx context.Context, cfg config) error {
			logStep("List pins\n")

			code, header, data, err := gatewayPost(
				ctx,
				cfg,
				"api/v0/pin/ls?quite=false&stream=false",
				nil,
				nil,
			)
			if err := logResp(code, header, data, err); err != nil {
				return err
			}

			res := pin.PinLsOutputWrapper{}
			if err := json.Unmarshal(data, &res); err != nil {
				return err
			}

			logStep("Unpins\n")
			for k, v := range res.Keys {
				if v.Type == "indirect" {
					continue
				}

				code, header, data, err = gatewayPost(
					ctx,
					cfg,
					fmt.Sprintf("api/v0/pin/rm?arg=%s", k),
					nil,
					nil,
				)
				if err := logResp(code, header, data, err); err != nil {
					return err
				}
			}

			return nil
		},
	},
	// "POST /api/v0/pin/add" pins and unpins an uploaded CID.
	&check{
		desc: "API: POST data, pin twice, verify and unpin",
		run: func(ctx context.Context, cfg config) error {
			logStep("Post data\n")

			code, header, data, err := gatewayPost(
				ctx,
				cfg,
				"ipfs/",
				nil,
				strings.NewReader(fmt.Sprintf(
					"Photon Gateway pinning Test - %v",
					time.Now().Format(time.RFC822Z),
				)),
			)
			if err := logResp(code, header, data, err); err != nil {
				return err
			}

			k := header.Get("Ipfs-Hash")
			if k == "" {
				return fmt.Errorf("CID missing")
			}

			for iter := 0; iter < 1; iter++ {
				logStep("Pin data, iter = %v\n", iter)
				done := false
				for !done {
					select {
					case <-ctx.Done():
						return ctx.Err()
					default:
						code, header, data, err := gatewayPost(
							ctx,
							cfg,
							fmt.Sprintf("api/v0/pin/add?arg=%s&progress=true", k),
							nil,
							nil,
						)
						if err := logResp(code, header, data, err); err != nil {
							return err
						}

						for _, line := range strings.Split(string(data), "\n") {
							line = strings.TrimSpace(line)
							if len(line) == 0 {
								continue
							}

							res := pin.AddPinOutput{}
							if err := json.Unmarshal(
								[]byte(line),
								&res,
							); err != nil {
								return err
							}

							if len(res.Pins) > 0 {
								logStep("Pin complete\n")
								done = true
							}
							logStep("Pin in progress %v\n", res.Progress)
						}
					}
				}
			}

			logStep("Verify CID\n")

			code, header, data, err = gatewayPost(
				ctx,
				cfg,
				fmt.Sprintf("api/v0/pin/verify?verbose=true"),
				nil,
				nil,
			)
			if err := logResp(code, header, data, err); err != nil {
				return err
			}

			logStep("Unpin\n")
			code, header, data, err = gatewayPost(
				ctx,
				cfg,
				fmt.Sprintf("api/v0/pin/rm?arg=%s", k),
				nil,
				nil,
			)
			return logResp(code, header, data, err)
		},
	},
	// "POST /api/v0/pin" pins and unpins an external CID.
	&check{
		desc: "API: pin and unpin external CID",
		run: func(ctx context.Context, cfg config) error {
			k := "QmP8jTG1m9GSDJLCbeWhVSVgEzCPPwXRdCRuJtQ5Tz9Kc9"

			logStep("Pin external CID\n")
			done := false
			for !done {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
					code, header, data, err := gatewayPost(
						ctx,
						cfg,
						fmt.Sprintf(
							"api/v0/pin/add?arg=%s&recursive=true&progress=true",
							k,
						),
						nil,
						nil,
					)
					if err := logResp(code, header, data, err); err != nil {
						return err
					}

					for _, line := range strings.Split(string(data), "\n") {
						line = strings.TrimSpace(line)
						if len(line) == 0 {
							continue
						}

						res := pin.AddPinOutput{}
						if err := json.Unmarshal(
							[]byte(line),
							&res,
						); err != nil {
							return err
						}

						if len(res.Pins) > 0 {
							logStep("Pin complete\n")
							done = true
						}
						logStep("Pin in progress %v\n", res.Progress)
					}
				}
			}

			logStep("Unpin\n")
			code, header, data, err := gatewayPost(
				ctx,
				cfg,
				fmt.Sprintf("api/v0/pin/rm?arg=%s", k),
				nil,
				nil,
			)
			return logResp(code, header, data, err)
		},
	},
	// "POST /api/v0/dag/xxxx" DAG operations: put/get/stat/import/export
	&check{
		desc: "API: DAG operations",
		run: func(ctx context.Context, cfg config) error {
			logStep("Put DAG leaves\n")

			nd0, _ := merkledag.NewRawNode([]byte("DAG Test - File0")).
				MarshalJSON()
			nd1, _ := merkledag.NewRawNode([]byte("DAG Test - File1")).
				MarshalJSON()
			nd2, _ := merkledag.NewRawNode([]byte("DAG Test - File2")).
				MarshalJSON()
			r := ipfsfiles.NewMultiFileReader(
				ipfsfiles.NewMapDirectory(map[string]ipfsfiles.Node{
					"dag_file0.txt": ipfsfiles.NewBytesFile(nd0),
					"dag_file1.txt": ipfsfiles.NewBytesFile(nd1),
					"dag_file2.txt": ipfsfiles.NewBytesFile(nd2),
				}),
				true,
			)

			header := http.Header{}
			header.Set("Content-Type", "multipart/form-data; boundary="+r.Boundary())
			header.Set("Content-Disposition", "form-data; name=\"files\"")

			code, header, data, err := gatewayPost(
				ctx,
				cfg,
				"api/v0/dag/put?input-codec=dag-json&allow-big-block=false",
				header,
				r,
			)
			if err := logResp(code, header, data, err); err != nil {
				return err
			}

			logStep("Stat DAG node & build the root node\n")

			root := merkledag.NodeWithData([]byte("DAG Test - Root"))
			idx := 0
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if len(line) == 0 {
					continue
				}

				obj := dagcmd.OutputObject{}
				if err := json.Unmarshal(
					[]byte(line),
					&obj,
				); err != nil {
					return err
				}

				code, header, data, err = gatewayPost(
					ctx,
					cfg,
					fmt.Sprintf("api/v0/dag/stat?arg=%v&progress=false", obj.Cid.String()),
					header,
					nil,
				)
				if err := logResp(code, header, data, err); err != nil {
					return err
				}

				stat := dagcmd.DagStat{}
				if err := json.Unmarshal(data, &stat); err != nil {
					return err
				}

				name := fmt.Sprintf("dag_file%v.txt", idx)
				root.AddRawLink(name, &format.Link{
					Name: name,
					Size: stat.Size,
					Cid:  obj.Cid,
				})
				idx++
			}

			logStep("Put DAG root\n")

			jsonRoot, _ := root.MarshalJSON()
			r = ipfsfiles.NewMultiFileReader(
				ipfsfiles.NewMapDirectory(map[string]ipfsfiles.Node{
					"root": ipfsfiles.NewBytesFile(jsonRoot),
				}),
				true,
			)

			header = http.Header{}
			header.Set("Content-Type", "multipart/form-data; boundary="+r.Boundary())
			header.Set("Content-Disposition", "form-data; name=\"files\"")

			code, header, data, err = gatewayPost(
				ctx,
				cfg,
				"api/v0/dag/put?input-codec=dag-json&allow-big-block=false",
				header,
				r,
			)
			if err := logResp(code, header, data, err); err != nil {
				return err
			}
			obj := dagcmd.OutputObject{}
			if err := json.Unmarshal(data, &obj); err != nil {
				return err
			}

			logStep("Get DAG node\n")

			code, header, data, err = gatewayPost(
				ctx,
				cfg,
				fmt.Sprintf("api/v0/dag/get?arg=%v", obj.Cid.String()),
				header,
				nil,
			)
			if err := logResp(code, header, data, err); err != nil {
				return err
			}

			logStep("Export DAG tree as a CAR file\n")

			code, header, data, err = gatewayPost(
				ctx,
				cfg,
				fmt.Sprintf("api/v0/dag/export?arg=%v&progress=false", obj.Cid.String()),
				header,
				nil,
			)
			if err := logResp(code, header, data, err); err != nil {
				return err
			}

			logStep("Import DAG tree from a CAR file\n")

			r = ipfsfiles.NewMultiFileReader(
				ipfsfiles.NewMapDirectory(map[string]ipfsfiles.Node{
					"path": ipfsfiles.NewBytesFile(data),
				}),
				true,
			)
			header = http.Header{}
			header.Set("Content-Type", "multipart/form-data; boundary="+r.Boundary())
			header.Set("Content-Disposition", "form-data; name=\"files\"")
			code, header, data, err = gatewayPost(
				ctx,
				cfg,
				"api/v0/dag/import?pin-roots=false",
				header,
				r,
			)
			if err := logResp(code, header, data, err); err != nil {
				return err
			}

			return nil
		},
	},
	// "POST /api/v0/files/xxxx" files operations:
	// ls/mkdir/read/write/rm/mv/cp/flush
	&check{
		desc: "API: MFS operations",
		run: func(ctx context.Context, cfg config) error {
			logStep("Write file0\n")

			r := ipfsfiles.NewMultiFileReader(
				ipfsfiles.NewMapDirectory(map[string]ipfsfiles.Node{
					"path": ipfsfiles.NewBytesFile([]byte(
						"MFS Test: File0",
					)),
				}),
				true,
			)
			header := http.Header{}
			header.Set("Content-Type", "multipart/form-data; boundary="+r.Boundary())
			header.Set("Content-Disposition", "form-data; name=\"files\"")
			code, header, data, err := gatewayPost(
				ctx,
				cfg,
				"api/v0/files/write?arg=/mfs_file0.txt&create=true&truncate=true&&parents=true",
				header,
				r,
			)
			if err := logResp(code, header, data, err); err != nil {
				return err
			}

			logStep("Overwrite partial file0\n")
			r = ipfsfiles.NewMultiFileReader(
				ipfsfiles.NewMapDirectory(map[string]ipfsfiles.Node{
					"path": ipfsfiles.NewBytesFile([]byte(
						"file0",
					)),
				}),
				true,
			)
			header = http.Header{}
			header.Set("Content-Type", "multipart/form-data; boundary="+r.Boundary())
			header.Set("Content-Disposition", "form-data; name=\"files\"")
			code, header, data, err = gatewayPost(
				ctx,
				cfg,
				"api/v0/files/write?arg=/mfs_file0.txt&offset=10&create=false&truncate=false",
				header,
				r,
			)
			if err := logResp(code, header, data, err); err != nil {
				return err
			}

			logStep("Write file1\n")
			r = ipfsfiles.NewMultiFileReader(
				ipfsfiles.NewMapDirectory(map[string]ipfsfiles.Node{
					"path": ipfsfiles.NewBytesFile([]byte(
						"MFS Test: File1",
					)),
				}),
				true,
			)
			header = http.Header{}
			header.Set("Content-Type", "multipart/form-data; boundary="+r.Boundary())
			header.Set("Content-Disposition", "form-data; name=\"files\"")
			code, header, data, err = gatewayPost(
				ctx,
				cfg,
				"api/v0/files/write?arg=/a_dir/mfs_file1.txt&create=true&truncate=true&parents=true",
				header,
				r,
			)
			if err := logResp(code, header, data, err); err != nil {
				return err
			}

			logStep("Read file0\n")
			code, header, data, err = gatewayPost(
				ctx,
				cfg,
				"api/v0/files/read?arg=/mfs_file0.txt",
				header,
				r,
			)
			if err := logResp(code, header, data, err); err != nil {
				return err
			}
			if string(data) != "MFS Test: file0" {
				return fmt.Errorf("unexpected read content\n")
			}

			logStep("Read partial file0\n")
			code, header, data, err = gatewayPost(
				ctx,
				cfg,
				"api/v0/files/read?arg=/mfs_file0.txt&offset=10&count=5",
				header,
				r,
			)
			if err := logResp(code, header, data, err); err != nil {
				return err
			}
			if string(data) != "file0" {
				return fmt.Errorf("unexpected read content\n")
			}

			logStep("Read partial file1\n")
			code, header, data, err = gatewayPost(
				ctx,
				cfg,
				"api/v0/files/read?arg=/a_dir/mfs_file1.txt&offset=10&count=5",
				header,
				r,
			)
			if err := logResp(code, header, data, err); err != nil {
				return err
			}
			if string(data) != "File1" {
				return fmt.Errorf("unexpected read content\n")
			}

			logStep("Mkdir\n")
			code, header, data, err = gatewayPost(
				ctx,
				cfg,
				"api/v0/files/mkdir?arg=/a_dir/child_dir",
				header,
				r,
			)
			if err := logResp(code, header, data, err); err != nil {
				return err
			}

			logStep("List dir\n")
			code, header, data, err = gatewayPost(
				ctx,
				cfg,
				"api/v0/files/ls?arg=/a_dir/",
				header,
				r,
			)
			if err := logResp(code, header, data, err); err != nil {
				return err
			}

			res := struct {
				Entries []mfs.NodeListing
			}{}
			if err := json.Unmarshal(data, &res); err != nil {
				return err
			}

			if len(res.Entries) != 2 ||
				res.Entries[0].Name != "child_dir" ||
				res.Entries[1].Name != "mfs_file1.txt" {
				return fmt.Errorf("unexpected MFS listing result")
			}

			logStep("Move\n")
			code, header, data, err = gatewayPost(
				ctx,
				cfg,
				"api/v0/files/mv?arg=/a_dir/child_dir&arg=/a_dir/dir_to_del",
				header,
				r,
			)
			if err := logResp(code, header, data, err); err != nil {
				return err
			}

			logStep("Copy\n")
			code, header, data, err = gatewayPost(
				ctx,
				cfg,
				"api/v0/files/mv?arg=/a_dir/mfs_file1.txt&arg=/a_dir/dir_to_del/file_to_del.txt",
				header,
				r,
			)
			if err := logResp(code, header, data, err); err != nil {
				return err
			}

			logStep("Remove\n")
			code, header, data, err = gatewayPost(
				ctx,
				cfg,
				"api/v0/files/rm?arg=/a_dir/dir_to_del&recursive=true",
				header,
				r,
			)
			if err := logResp(code, header, data, err); err != nil {
				return err
			}

			return nil
		},
	},
}
