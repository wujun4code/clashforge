//go:build tools

package mihomobridge

// Pins golang.org/x/mobile in the module graph so `gomobile bind` on CI
// works against the committed go.mod/go.sum without network-time `go get`.
import _ "golang.org/x/mobile/bind"
