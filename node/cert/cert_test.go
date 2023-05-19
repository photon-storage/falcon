package cert

import (
	"fmt"
	"testing"

	"github.com/photon-storage/go-common/testing/require"
)

func TestObtainCert(t *testing.T) {
	t.Skip()

	sk, cert, err := ObtainCert("jump@gw3.io", "jump.gw3.io", nil)
	require.NoError(t, err)
	fmt.Printf("%s\n", string(sk))
	fmt.Printf("%s\n", string(cert))
}
