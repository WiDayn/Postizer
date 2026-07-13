package main

import (
	"context"
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
)

const backupFilenamePrefix = "postizer-export-"

type webDAVClient struct {
	base       *url.URL
	remotePath string
	username   string
	password   string
	http       *http.Client
}

func newWebDAVClient(cfg config) (*webDAVClient, error) {
	base, err := url.Parse(cfg.ServerURL)
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.SkipTLSVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec -- explicit administrator option for private WebDAV servers
	}
	return &webDAVClient{
		base:       base,
		remotePath: strings.Trim(cfg.RemotePath, "/"),
		username:   cfg.Username,
		password:   cfg.Password,
		http: &http.Client{
			Transport: transport,
			Timeout:   0,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (c *webDAVClient) endpoint(relative string) string {
	u := *c.base
	joined := strings.TrimSuffix(u.Path, "/")
	for _, part := range []string{c.remotePath, relative} {
		if strings.Trim(part, "/") != "" {
			joined = path.Join(joined, strings.Trim(part, "/"))
		}
	}
	if joined == "" {
		joined = "/"
	}
	u.Path = joined
	u.RawPath = ""
	return u.String()
}

func (c *webDAVClient) request(ctx context.Context, method, target string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, err
	}
	if c.username != "" || c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	return c.http.Do(req)
}

func (c *webDAVClient) ensureCollection(ctx context.Context) error {
	parts := strings.Split(strings.Trim(c.remotePath, "/"), "/")
	for index := range parts {
		relative := strings.Join(parts[:index+1], "/")
		u := *c.base
		u.Path = path.Join(strings.TrimSuffix(c.base.Path, "/"), relative)
		response, err := c.request(ctx, "MKCOL", u.String(), nil)
		if err != nil {
			return fmt.Errorf("创建 WebDAV 目录 %q: %w", relative, err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusMethodNotAllowed && response.StatusCode != http.StatusOK {
			return fmt.Errorf("创建 WebDAV 目录 %q: %s", relative, response.Status)
		}
	}
	return nil
}

func (c *webDAVClient) upload(ctx context.Context, localPath, filename string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "PUT", c.endpoint(filename), file)
	if err != nil {
		return err
	}
	req.ContentLength = info.Size()
	req.Header.Set("Content-Type", "application/zip")
	if c.username != "" || c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	response, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("上传 WebDAV 备份: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		return fmt.Errorf("上传 WebDAV 备份: %s: %s", response.Status, strings.TrimSpace(string(message)))
	}
	return nil
}

type multiStatus struct {
	Responses []struct {
		Href string `xml:"href"`
	} `xml:"response"`
}

func (c *webDAVClient) listBackups(ctx context.Context) ([]string, error) {
	body := strings.NewReader(`<?xml version="1.0"?><d:propfind xmlns:d="DAV:"><d:prop><d:resourcetype/></d:prop></d:propfind>`)
	req, err := http.NewRequestWithContext(ctx, "PROPFIND", c.endpoint(""), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Depth", "1")
	req.Header.Set("Content-Type", "application/xml")
	if c.username != "" || c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	response, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusMultiStatus && (response.StatusCode < 200 || response.StatusCode >= 300) {
		return nil, fmt.Errorf("列出 WebDAV 备份: %s", response.Status)
	}
	var listing multiStatus
	if err := xml.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&listing); err != nil {
		return nil, fmt.Errorf("解析 WebDAV 目录响应: %w", err)
	}
	var names []string
	for _, item := range listing.Responses {
		u, err := url.Parse(item.Href)
		if err != nil {
			continue
		}
		name, err := url.PathUnescape(path.Base(strings.TrimSuffix(u.Path, "/")))
		if err != nil {
			continue
		}
		if strings.HasPrefix(name, backupFilenamePrefix) && strings.HasSuffix(name, ".zip") {
			names = append(names, name)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	return names, nil
}

func (c *webDAVClient) prune(ctx context.Context, keep int) error {
	names, err := c.listBackups(ctx)
	if err != nil {
		return err
	}
	if len(names) <= keep {
		return nil
	}
	for _, name := range names[keep:] {
		response, err := c.request(ctx, "DELETE", c.endpoint(name), nil)
		if err != nil {
			return err
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusNoContent && response.StatusCode != http.StatusOK && response.StatusCode != http.StatusNotFound {
			return fmt.Errorf("删除旧备份 %q: %s", name, response.Status)
		}
	}
	return nil
}
