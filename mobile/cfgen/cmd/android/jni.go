//go:build android

// Package main is the Android JNI entry point for the cfgen shared library.
// Build with:
//
//	CGO_ENABLED=1 GOOS=android GOARCH=arm64 CC=<ndk-clang> \
//	  go build -buildmode=c-shared -o libcfgen.so ./cmd/android/
package main

/*
#include <jni.h>
#include <stdlib.h>

// Thin wrappers so CGo can call through the JNIEnv function-pointer table
// without having to dereference it from Go directly.
static jstring new_string_utf(JNIEnv *env, const char *utf) {
	return (*env)->NewStringUTF(env, utf);
}
static const char *get_string_utf_chars(JNIEnv *env, jstring s) {
	return (*env)->GetStringUTFChars(env, s, NULL);
}
static void release_string_utf_chars(JNIEnv *env, jstring s, const char *chars) {
	(*env)->ReleaseStringUTFChars(env, s, chars);
}
*/
import "C"
import (
	"unsafe"

	cfgen "github.com/wujun4code/clashforge/mobile/cfgen"
)

// Java_com_clashforge_mobile_clashforge_1mobile_ConfigGen_nativeProbeAndPatchDNS
// is called before the VPN interface is established so probe sockets travel
// the physical network.  Returns a JSON string:
//
//	{"summary":"dns-probe: ...","was_patched":true}
//
//export Java_com_clashforge_mobile_clashforge_1mobile_ConfigGen_nativeProbeAndPatchDNS
func Java_com_clashforge_mobile_clashforge_1mobile_ConfigGen_nativeProbeAndPatchDNS(
	env *C.JNIEnv, _ C.jclass, jConfigPath C.jstring,
) C.jstring {
	configPath := jstr(env, jConfigPath)
	result := cfgen.ProbeAndPatchDNS(configPath)
	return tojstr(env, cfgen.MarshalProbeResult(result))
}

// Java_com_clashforge_mobile_clashforge_1mobile_ConfigGen_nativeGenerateConfig
// reads the (possibly probe-patched) subscription YAML at configPath and writes
// the final Android-ready config back to the same path.
// tunFd is the fd number to embed in tun.file-descriptor (always 0 on Android
// because startMihomoCore dup2s the real TUN fd onto stdin before forking).
// dnsMode: "fake-ip" or "redir-host" (empty string → "fake-ip").
// Returns JSON: {"ok":true,"error":""}
//
//export Java_com_clashforge_mobile_clashforge_1mobile_ConfigGen_nativeGenerateConfig
func Java_com_clashforge_mobile_clashforge_1mobile_ConfigGen_nativeGenerateConfig(
	env *C.JNIEnv, _ C.jclass,
	jConfigPath C.jstring, jTunFd C.jint, jGeoDataDir C.jstring, jDnsMode C.jstring,
) C.jstring {
	configPath := jstr(env, jConfigPath)
	tunFd := int(jTunFd)
	geoDataDir := jstr(env, jGeoDataDir)
	dnsMode := jstr(env, jDnsMode)
	result := cfgen.GenerateConfig(configPath, tunFd, geoDataDir, dnsMode)
	return tojstr(env, cfgen.MarshalGenerateResult(result))
}

// ── JNI string helpers ───────────────────────────────────────────────────────

func jstr(env *C.JNIEnv, s C.jstring) string {
	if s == 0 {
		return ""
	}
	c := C.get_string_utf_chars(env, s)
	if c == nil {
		return ""
	}
	defer C.release_string_utf_chars(env, s, c)
	return C.GoString(c)
}

func tojstr(env *C.JNIEnv, s string) C.jstring {
	c := C.CString(s)
	defer C.free(unsafe.Pointer(c))
	return C.new_string_utf(env, c)
}

func main() {}
