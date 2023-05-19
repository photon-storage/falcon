# Falcon

Falcon is a port of the official IPFS node implementation [Kubo](https://github.com/ipfs/kubo).
It is one critical component of the [Gateway3](www.gw3.io) project, which tries to provide a decentralized IPFS gateway alternative.
The falcon node is able to run by its own and acts like a normal IPFS node.
However, it needs to join the [Gateway3](www.gw3.io) protocol in order to make its service accessible from the world.

Command to start the falcon service:
```
go run ./cmd/falcon/. daemon --init --falcon-config=./cmd/falcon/config/config_dev.yaml
```
