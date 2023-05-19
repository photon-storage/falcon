package consts

type AcceptMediaType string

const (
	AcceptJson              AcceptMediaType = "application/json"
	AcceptCbor              AcceptMediaType = "application/cbor"
	AcceptXTar              AcceptMediaType = "application/x-tar"
	AcceptVndIpldRaw        AcceptMediaType = "application/vnd.ipld.raw"
	AcceptVndIpldCar        AcceptMediaType = "application/vnd.ipld.car"
	AcceptVndIpldDagJson    AcceptMediaType = "application/vnd.ipld.dag-json"
	AcceptVndIpldDagCbor    AcceptMediaType = "application/vnd.ipld.dag-cbor"
	AcceptVndIpfsIpnsRecord AcceptMediaType = "application/vnd.ipfs.ipns-record"
)
