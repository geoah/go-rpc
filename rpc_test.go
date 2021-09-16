package rpc

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

type (
	Math       struct{}
	AddRequest struct {
		A int
		B int
	}
	AddResponse struct {
		X int
	}
)

func (m *Math) Add(req *AddRequest, res *AddResponse) error {
	res.X = req.A + req.B
	return nil
}

func TestService_Integration(t *testing.T) {
	// Start http server.
	go func() {
		err := http.ListenAndServe("localhost:10123", nil)
		require.NoError(t, err)
	}()

	// Create service.
	s := New()

	// Register methods.
	err := s.Register(&Math{})
	require.NoError(t, err)

	// Register handler.
	http.Handle("/rpc", s.Serve())

	// Call Math.Add.
	req := &AddRequest{A: 1, B: 2}
	res := &AddResponse{}
	err = s.Call(http.DefaultClient, "http://localhost:10123/rpc", "Math.Add", req, res)
	require.NoError(t, err)
	require.Equal(t, 3, res.X)
}
