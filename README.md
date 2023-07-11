# Falcon

Falcon is a port of the official IPFS node implementation [Kubo](https://github.com/ipfs/kubo).
It is one critical component of the [Gateway3](https://www.gw3.io) project, which tries to provide a decentralized IPFS gateway alternative.
The falcon node is able to run by its own and acts like a normal IPFS node.
However, it needs to join the [Gateway3](https://www.gw3.io) protocol in order to make its service accessible from the world.

# How to Run
Run `make` under the repo directory to start Falcon node in loner mode.
The loner mode is the same as prod or dev mode except it does not communicate with other [Gateway3](https://www.gw3.io) services.
In this mode, you can test Falcon's API using end-to-end checks by running `make checks`.
The end-to-end checks report successes or failures for varioues APIs.
It is recommended to run checks against changes made to node.

Run `make prod` or `make dev` to start other modes.
