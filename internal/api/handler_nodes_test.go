package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/wujun4code/clashforge/internal/nodes"
)

// newNodeStoreWithNode is a test helper that creates a temp-dir NodeStore and inserts a node.
func newNodeStoreWithNode(t *testing.T, n *nodes.Node) (*nodes.Store, string) {
	t.Helper()
	store, err := nodes.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.Create(n); err != nil {
		t.Fatalf("Create node: %v", err)
	}
	return store, n.ID
}

// deployRequest POSTs to /nodes/{id}/deploy and returns the response.
func deployRequest(t *testing.T, store *nodes.Store, nodeID, mode string) *httptest.ResponseRecorder {
	t.Helper()

	body, _ := json.Marshal(map[string]string{"mode": mode})
	req := httptest.NewRequest(http.MethodPost, "/nodes/"+nodeID+"/deploy", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", nodeID)
	req = req.WithContext(chi.NewContext(req.Context(), rctx))

	rr := httptest.NewRecorder()
	// handleDeployNode requires both store and kp; pass nil kp — validation fires before SSH
	handleDeployNode(store, nil)(rr, req)
	return rr
}

// TestHandleDeployNode_FullModeRequiresAllCFFields verifies that requesting a full
// deploy on a node that is missing domain / email / cf_token returns HTTP 400 with
// the right error code before any SSH connection is attempted.
func TestHandleDeployNode_FullModeRequiresAllCFFields(t *testing.T) {
	cases := []struct {
		name   string
		node   nodes.Node
		wantOK bool
	}{
		{
			name:   "domain missing → 400",
			node:   nodes.Node{Name: "n", Host: "1.2.3.4", Port: 22, Username: "root", Email: "ops@x.com", CFToken: "tok"},
			wantOK: false,
		},
		{
			name:   "email missing → 400",
			node:   nodes.Node{Name: "n", Host: "1.2.3.4", Port: 22, Username: "root", Domain: "edge.example.com", CFToken: "tok"},
			wantOK: false,
		},
		{
			name:   "cf_token missing → 400",
			node:   nodes.Node{Name: "n", Host: "1.2.3.4", Port: 22, Username: "root", Domain: "edge.example.com", Email: "ops@x.com"},
			wantOK: false,
		},
		{
			name: "all fields set → reaches SSH (fails with non-400 status)",
			node: nodes.Node{
				Name: "n", Host: "1.2.3.4", Port: 22, Username: "root",
				Domain: "edge.example.com", Email: "ops@x.com", CFToken: "tok",
			},
			// The handler will attempt SSE+SSH and fail beyond validation, so the
			// response won't be 400 (it will be 500 "SSE not supported" in test context).
			wantOK: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, id := newNodeStoreWithNode(t, &tc.node)
			rr := deployRequest(t, store, id, "full")

			if !tc.wantOK {
				if rr.Code != http.StatusBadRequest {
					t.Errorf("expected 400, got %d — body: %s", rr.Code, rr.Body.String())
				}
				var resp APIResponse
				if err := json.NewDecoder(rr.Body).Decode(&resp); err == nil {
					if resp.Error == nil || resp.Error.Code != "NODE_DEPLOY_FULL_PARAMS_REQUIRED" {
						t.Errorf("expected NODE_DEPLOY_FULL_PARAMS_REQUIRED, got: %+v", resp.Error)
					}
				}
			} else {
				if rr.Code == http.StatusBadRequest {
					t.Errorf("unexpected 400 when all CF fields are present — body: %s", rr.Body.String())
				}
			}
		})
	}
}

// TestHandleDeployNode_NotFound verifies HTTP 404 when the node ID does not exist.
func TestHandleDeployNode_NotFound(t *testing.T) {
	store, err := nodes.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	rr := deployRequest(t, store, "nonexistent-id", "full")
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

// TestHandleDeployNode_Bootstrap does not require CF fields — bootstrap mode skips
// domain/email/token validation entirely.
func TestHandleDeployNode_Bootstrap(t *testing.T) {
	n := nodes.Node{Name: "n", Host: "1.2.3.4", Port: 22, Username: "root"}
	store, id := newNodeStoreWithNode(t, &n)
	rr := deployRequest(t, store, id, "bootstrap")
	// Should not 400 on validation; it will fail at SSE or SSH, never at the CF param check
	if rr.Code == http.StatusBadRequest {
		t.Errorf("bootstrap deploy should not fail CF-params validation, got 400: %s", rr.Body.String())
	}
}
