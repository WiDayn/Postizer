package pluginhost

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"postizer/internal/appearance"
	"postizer/pkg/pluginrpc"
)

func TestHostStartsGRPCPluginAndInvokesAction(t *testing.T) {
	host := New(filepath.Join("..", ".."))
	defer host.Close()
	pluginRoot := filepath.Join("..", "..", "examples", "bundles", "wordpress-importer", "plugins", "wordpress-importer")

	pack := appearance.Pack{
		Manifest: appearance.Manifest{
			ID:      "wordpress-importer",
			Type:    appearance.PluginPack,
			Name:    "WordPress Importer",
			Version: "1.0.0",
			Runtime: appearance.PluginRuntime{
				Kind:    appearance.RuntimeGRPC,
				Command: "${go}",
				Args: []string{
					"run",
					"./cmd/postizer-wordpress-importer",
				},
			},
		},
		RootDir: pluginRoot,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	result, err := host.InvokeAction(ctx, pack, inspectRequest())
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Title, "WordPress export inspected") {
		t.Fatalf("unexpected action result: %#v", result)
	}

	time.Sleep(200 * time.Millisecond)

	ctx, cancel = context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	result, err = host.InvokeAction(ctx, pack, inspectRequest())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Title, "WordPress export inspected") {
		t.Fatalf("unexpected action result: %#v", result)
	}
}

func inspectRequest() *pluginrpc.InvokeActionRequest {
	return &pluginrpc.InvokeActionRequest{
		PluginID: "wordpress-importer",
		ActionID: "inspect_wxr",
		Files: []pluginrpc.ActionFile{
			{
				Name:        "wxr_file",
				Filename:    "export.xml",
				ContentType: "application/xml",
				Body:        []byte(`<?xml version="1.0"?><rss><channel><title>Fixture</title><item><title>Hello</title><wp:post_type xmlns:wp="http://wordpress.org/export/1.2/">post</wp:post_type><wp:status xmlns:wp="http://wordpress.org/export/1.2/">publish</wp:status></item></channel></rss>`),
			},
		},
	}
}

func TestHostSelectsPlatformRuntimeCommand(t *testing.T) {
	root := t.TempDir()
	host := New(root)
	pack := appearance.Pack{
		Manifest: appearance.Manifest{
			ID: "binary-plugin",
			Runtime: appearance.PluginRuntime{
				Kind:    appearance.RuntimeGRPC,
				Command: "fallback",
				Args:    []string{"fallback-arg"},
				Env: map[string]string{
					"BASE": "base",
				},
				Platforms: []appearance.PluginRuntimePlatform{
					{
						GOOS:    runtime.GOOS,
						GOArch:  runtime.GOARCH,
						Command: "bin/" + runtime.GOOS + "-" + runtime.GOARCH + "/plugin",
						Args:    []string{"platform-arg"},
						Env: map[string]string{
							"PLATFORM": "selected",
						},
					},
				},
			},
		},
		RootDir: filepath.Join(root, "pack"),
	}

	selected, err := host.command(pack)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := selected.command, "bin/"+runtime.GOOS+"-"+runtime.GOARCH+"/plugin"; got != want {
		t.Fatalf("command = %q, want %q", got, want)
	}
	if got, want := strings.Join(selected.args, ","), "platform-arg"; got != want {
		t.Fatalf("args = %q, want %q", got, want)
	}
	if got, want := selected.env["BASE"], "base"; got != want {
		t.Fatalf("base env = %q, want %q", got, want)
	}
	if got, want := selected.env["PLATFORM"], "selected"; got != want {
		t.Fatalf("platform env = %q, want %q", got, want)
	}

	resolved := host.resolveCommand(pack, selected.command)
	want, err := filepath.Abs(filepath.Join(pack.RootDir, "bin", runtime.GOOS+"-"+runtime.GOARCH, "plugin"))
	if err != nil {
		t.Fatal(err)
	}
	if resolved != want {
		t.Fatalf("resolved command = %q, want %q", resolved, want)
	}
}

func TestHostResolvesRelativeRuntimePathsToAbsolutePaths(t *testing.T) {
	root := t.TempDir()
	host := New(".")
	pack := appearance.Pack{
		Manifest: appearance.Manifest{ID: "binary-plugin"},
		RootDir:  filepath.Join(root, "content", "bundles", "plugin"),
	}

	command := host.resolveCommand(pack, "bin/windows-amd64/plugin.exe")
	if !filepath.IsAbs(command) {
		t.Fatalf("resolved command should be absolute, got %q", command)
	}
	if got, want := command, filepath.Join(pack.RootDir, "bin", "windows-amd64", "plugin.exe"); got != want {
		t.Fatalf("resolved command = %q, want %q", got, want)
	}

	workDir := host.workDir(pack, "")
	if !filepath.IsAbs(workDir) {
		t.Fatalf("work dir should be absolute, got %q", workDir)
	}
	if got, want := workDir, pack.RootDir; got != want {
		t.Fatalf("work dir = %q, want %q", got, want)
	}
}
