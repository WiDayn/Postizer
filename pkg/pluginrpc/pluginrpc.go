package pluginrpc

import (
	"context"
	"encoding/json"

	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding"
)

const (
	ProtocolVersion = "postizer-plugin-grpc-json-v1"
	ServiceName     = "postizer.plugin.v1.PluginService"
	HostServiceName = "postizer.plugin.v1.HostService"
)

var Codec = jsonCodec{}

func init() {
	encoding.RegisterCodec(Codec)
}

type jsonCodec struct{}

func (jsonCodec) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (jsonCodec) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func (jsonCodec) Name() string {
	return "json"
}

type HandshakeRequest struct {
	ProtocolVersion string `json:"protocol_version"`
	HostVersion     string `json:"host_version"`
	PluginID        string `json:"plugin_id"`
}

type HandshakeResponse struct {
	ProtocolVersion string `json:"protocol_version"`
	PluginID        string `json:"plugin_id"`
	PluginVersion   string `json:"plugin_version"`
	Ready           bool   `json:"ready"`
	Message         string `json:"message,omitempty"`
}

type InvokeActionRequest struct {
	PluginID string            `json:"plugin_id"`
	ActionID string            `json:"action_id"`
	Fields   map[string]string `json:"fields,omitempty"`
	Files    []ActionFile      `json:"files,omitempty"`
}

type ActionFile struct {
	Name        string `json:"name"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Body        []byte `json:"body"`
}

type InvokeActionResponse struct {
	Title       string          `json:"title,omitempty"`
	Summary     string          `json:"summary,omitempty"`
	Level       string          `json:"level,omitempty"`
	Sections    []ResultSection `json:"sections,omitempty"`
	NextActions []NextAction    `json:"next_actions,omitempty"`
	Job         *ImportJob      `json:"job,omitempty"`
}

type ResultSection struct {
	Title string       `json:"title,omitempty"`
	Kind  string       `json:"kind,omitempty"`
	Text  string       `json:"text,omitempty"`
	Rows  []ResultRow  `json:"rows,omitempty"`
	Cards []ResultCard `json:"cards,omitempty"`
}

type ResultRow struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// ResultCard is a visual plugin result suitable for media, product, book, and
// other cover-driven collections. Actions are invoked through the same plugin
// action endpoint as ordinary next actions.
type ResultCard struct {
	ID          string       `json:"id,omitempty"`
	Title       string       `json:"title"`
	Subtitle    string       `json:"subtitle,omitempty"`
	Description string       `json:"description,omitempty"`
	ImageURL    string       `json:"image_url,omitempty"`
	URL         string       `json:"url,omitempty"`
	Badges      []string     `json:"badges,omitempty"`
	Actions     []NextAction `json:"actions,omitempty"`
}

type NextAction struct {
	ID      string            `json:"id"`
	Label   string            `json:"label"`
	Style   string            `json:"style,omitempty"`
	Confirm string            `json:"confirm,omitempty"`
	Fields  map[string]string `json:"fields,omitempty"`
}

type ContentDraft struct {
	Title   string   `json:"title"`
	Slug    string   `json:"slug"`
	Date    string   `json:"date"`
	Updated string   `json:"updated"`
	Tags    []string `json:"tags,omitempty"`
	Summary string   `json:"summary,omitempty"`
	Draft   bool     `json:"draft,omitempty"`
	TOC     bool     `json:"toc,omitempty"`
	Body    string   `json:"body"`
}

type ImportJob struct {
	ID       string          `json:"id"`
	Status   string          `json:"status"`
	Done     int             `json:"done"`
	Total    int             `json:"total"`
	Percent  int             `json:"percent"`
	Logs     []string        `json:"logs,omitempty"`
	Sections []ResultSection `json:"sections,omitempty"`
}

type CreateJobRequest struct {
	PluginID string `json:"plugin_id"`
	Title    string `json:"title,omitempty"`
	Total    int    `json:"total,omitempty"`
}

type UpdateJobRequest struct {
	PluginID string          `json:"plugin_id"`
	JobID    string          `json:"job_id"`
	Status   string          `json:"status,omitempty"`
	Done     int             `json:"done,omitempty"`
	Total    int             `json:"total,omitempty"`
	Log      string          `json:"log,omitempty"`
	Error    string          `json:"error,omitempty"`
	Sections []ResultSection `json:"sections,omitempty"`
}

type SaveMediaRequest struct {
	PluginID     string `json:"plugin_id"`
	OriginalName string `json:"original_name"`
	Alt          string `json:"alt,omitempty"`
	Caption      string `json:"caption,omitempty"`
	ContentType  string `json:"content_type,omitempty"`
	Body         []byte `json:"body"`
}

type MediaItem struct {
	ID           string `json:"id"`
	Path         string `json:"path"`
	OriginalName string `json:"original_name"`
	Alt          string `json:"alt,omitempty"`
	Caption      string `json:"caption,omitempty"`
	MIMEType     string `json:"mime_type,omitempty"`
	Width        int    `json:"width,omitempty"`
	Height       int    `json:"height,omitempty"`
}

type SaveMediaResponse struct {
	Item     MediaItem `json:"item"`
	Markdown string    `json:"markdown,omitempty"`
}

type SaveContentResponse struct {
	Title string `json:"title"`
	Slug  string `json:"slug"`
	URL   string `json:"url"`
}

type ReloadRuntimeRequest struct {
	PluginID string `json:"plugin_id"`
}

type ReloadRuntimeResponse struct {
	OK bool `json:"ok"`
}

type CreateContentExportRequest struct {
	PluginID string `json:"plugin_id"`
}

type CreateContentExportResponse struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	DownloadURL string `json:"download_url"`
	Bytes       int64  `json:"bytes"`
	Posts       int    `json:"posts"`
	Pages       int    `json:"pages"`
	MediaItems  int    `json:"media_items"`
	MediaFiles  int    `json:"media_files"`
}

type ShutdownRequest struct {
	PluginID string `json:"plugin_id"`
}

type ShutdownResponse struct {
	OK bool `json:"ok"`
}

type PluginServiceServer interface {
	Handshake(context.Context, *HandshakeRequest) (*HandshakeResponse, error)
	InvokeAction(context.Context, *InvokeActionRequest) (*InvokeActionResponse, error)
	Shutdown(context.Context, *ShutdownRequest) (*ShutdownResponse, error)
}

type PluginServiceClient interface {
	Handshake(context.Context, *HandshakeRequest, ...grpc.CallOption) (*HandshakeResponse, error)
	InvokeAction(context.Context, *InvokeActionRequest, ...grpc.CallOption) (*InvokeActionResponse, error)
	Shutdown(context.Context, *ShutdownRequest, ...grpc.CallOption) (*ShutdownResponse, error)
}

type HostServiceServer interface {
	CreateJob(context.Context, *CreateJobRequest) (*ImportJob, error)
	UpdateJob(context.Context, *UpdateJobRequest) (*ImportJob, error)
	SaveMedia(context.Context, *SaveMediaRequest) (*SaveMediaResponse, error)
	SavePost(context.Context, *ContentDraft) (*SaveContentResponse, error)
	SavePage(context.Context, *ContentDraft) (*SaveContentResponse, error)
	ReloadRuntime(context.Context, *ReloadRuntimeRequest) (*ReloadRuntimeResponse, error)
	CreateContentExport(context.Context, *CreateContentExportRequest) (*CreateContentExportResponse, error)
}

type HostServiceClient interface {
	CreateJob(context.Context, *CreateJobRequest, ...grpc.CallOption) (*ImportJob, error)
	UpdateJob(context.Context, *UpdateJobRequest, ...grpc.CallOption) (*ImportJob, error)
	SaveMedia(context.Context, *SaveMediaRequest, ...grpc.CallOption) (*SaveMediaResponse, error)
	SavePost(context.Context, *ContentDraft, ...grpc.CallOption) (*SaveContentResponse, error)
	SavePage(context.Context, *ContentDraft, ...grpc.CallOption) (*SaveContentResponse, error)
	ReloadRuntime(context.Context, *ReloadRuntimeRequest, ...grpc.CallOption) (*ReloadRuntimeResponse, error)
	CreateContentExport(context.Context, *CreateContentExportRequest, ...grpc.CallOption) (*CreateContentExportResponse, error)
}

type pluginServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewPluginServiceClient(cc grpc.ClientConnInterface) PluginServiceClient {
	return &pluginServiceClient{cc: cc}
}

type hostServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewHostServiceClient(cc grpc.ClientConnInterface) HostServiceClient {
	return &hostServiceClient{cc: cc}
}

func (c *pluginServiceClient) Handshake(ctx context.Context, in *HandshakeRequest, opts ...grpc.CallOption) (*HandshakeResponse, error) {
	out := new(HandshakeResponse)
	err := c.cc.Invoke(ctx, "/"+ServiceName+"/Handshake", in, out, append(defaultCallOptions(), opts...)...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *pluginServiceClient) InvokeAction(ctx context.Context, in *InvokeActionRequest, opts ...grpc.CallOption) (*InvokeActionResponse, error) {
	out := new(InvokeActionResponse)
	err := c.cc.Invoke(ctx, "/"+ServiceName+"/InvokeAction", in, out, append(defaultCallOptions(), opts...)...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *pluginServiceClient) Shutdown(ctx context.Context, in *ShutdownRequest, opts ...grpc.CallOption) (*ShutdownResponse, error) {
	out := new(ShutdownResponse)
	err := c.cc.Invoke(ctx, "/"+ServiceName+"/Shutdown", in, out, append(defaultCallOptions(), opts...)...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *hostServiceClient) CreateJob(ctx context.Context, in *CreateJobRequest, opts ...grpc.CallOption) (*ImportJob, error) {
	out := new(ImportJob)
	err := c.cc.Invoke(ctx, "/"+HostServiceName+"/CreateJob", in, out, append(defaultCallOptions(), opts...)...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *hostServiceClient) UpdateJob(ctx context.Context, in *UpdateJobRequest, opts ...grpc.CallOption) (*ImportJob, error) {
	out := new(ImportJob)
	err := c.cc.Invoke(ctx, "/"+HostServiceName+"/UpdateJob", in, out, append(defaultCallOptions(), opts...)...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *hostServiceClient) SaveMedia(ctx context.Context, in *SaveMediaRequest, opts ...grpc.CallOption) (*SaveMediaResponse, error) {
	out := new(SaveMediaResponse)
	err := c.cc.Invoke(ctx, "/"+HostServiceName+"/SaveMedia", in, out, append(defaultCallOptions(), opts...)...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *hostServiceClient) SavePost(ctx context.Context, in *ContentDraft, opts ...grpc.CallOption) (*SaveContentResponse, error) {
	out := new(SaveContentResponse)
	err := c.cc.Invoke(ctx, "/"+HostServiceName+"/SavePost", in, out, append(defaultCallOptions(), opts...)...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *hostServiceClient) SavePage(ctx context.Context, in *ContentDraft, opts ...grpc.CallOption) (*SaveContentResponse, error) {
	out := new(SaveContentResponse)
	err := c.cc.Invoke(ctx, "/"+HostServiceName+"/SavePage", in, out, append(defaultCallOptions(), opts...)...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *hostServiceClient) ReloadRuntime(ctx context.Context, in *ReloadRuntimeRequest, opts ...grpc.CallOption) (*ReloadRuntimeResponse, error) {
	out := new(ReloadRuntimeResponse)
	err := c.cc.Invoke(ctx, "/"+HostServiceName+"/ReloadRuntime", in, out, append(defaultCallOptions(), opts...)...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *hostServiceClient) CreateContentExport(ctx context.Context, in *CreateContentExportRequest, opts ...grpc.CallOption) (*CreateContentExportResponse, error) {
	out := new(CreateContentExportResponse)
	err := c.cc.Invoke(ctx, "/"+HostServiceName+"/CreateContentExport", in, out, append(defaultCallOptions(), opts...)...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func RegisterPluginServiceServer(s grpc.ServiceRegistrar, srv PluginServiceServer) {
	s.RegisterService(&grpc.ServiceDesc{
		ServiceName: ServiceName,
		HandlerType: (*PluginServiceServer)(nil),
		Methods: []grpc.MethodDesc{
			{MethodName: "Handshake", Handler: handshakeHandler},
			{MethodName: "InvokeAction", Handler: invokeActionHandler},
			{MethodName: "Shutdown", Handler: shutdownHandler},
		},
		Streams:  []grpc.StreamDesc{},
		Metadata: "pkg/pluginrpc/proto/plugin.proto",
	}, srv)
}

func RegisterHostServiceServer(s grpc.ServiceRegistrar, srv HostServiceServer) {
	s.RegisterService(&grpc.ServiceDesc{
		ServiceName: HostServiceName,
		HandlerType: (*HostServiceServer)(nil),
		Methods: []grpc.MethodDesc{
			{MethodName: "CreateJob", Handler: hostCreateJobHandler},
			{MethodName: "UpdateJob", Handler: hostUpdateJobHandler},
			{MethodName: "SaveMedia", Handler: hostSaveMediaHandler},
			{MethodName: "SavePost", Handler: hostSavePostHandler},
			{MethodName: "SavePage", Handler: hostSavePageHandler},
			{MethodName: "ReloadRuntime", Handler: hostReloadRuntimeHandler},
			{MethodName: "CreateContentExport", Handler: hostCreateContentExportHandler},
		},
		Streams:  []grpc.StreamDesc{},
		Metadata: "pkg/pluginrpc/proto/plugin.proto",
	}, srv)
}

func defaultCallOptions() []grpc.CallOption {
	return []grpc.CallOption{grpc.ForceCodec(Codec)}
}

func handshakeHandler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	in := new(HandshakeRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(PluginServiceServer).Handshake(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/" + ServiceName + "/Handshake"}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(PluginServiceServer).Handshake(ctx, req.(*HandshakeRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func invokeActionHandler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	in := new(InvokeActionRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(PluginServiceServer).InvokeAction(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/" + ServiceName + "/InvokeAction"}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(PluginServiceServer).InvokeAction(ctx, req.(*InvokeActionRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func shutdownHandler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	in := new(ShutdownRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(PluginServiceServer).Shutdown(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/" + ServiceName + "/Shutdown"}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(PluginServiceServer).Shutdown(ctx, req.(*ShutdownRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func hostCreateJobHandler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	in := new(CreateJobRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(HostServiceServer).CreateJob(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/" + HostServiceName + "/CreateJob"}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(HostServiceServer).CreateJob(ctx, req.(*CreateJobRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func hostUpdateJobHandler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	in := new(UpdateJobRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(HostServiceServer).UpdateJob(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/" + HostServiceName + "/UpdateJob"}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(HostServiceServer).UpdateJob(ctx, req.(*UpdateJobRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func hostSaveMediaHandler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	in := new(SaveMediaRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(HostServiceServer).SaveMedia(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/" + HostServiceName + "/SaveMedia"}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(HostServiceServer).SaveMedia(ctx, req.(*SaveMediaRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func hostSavePostHandler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	in := new(ContentDraft)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(HostServiceServer).SavePost(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/" + HostServiceName + "/SavePost"}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(HostServiceServer).SavePost(ctx, req.(*ContentDraft))
	}
	return interceptor(ctx, in, info, handler)
}

func hostSavePageHandler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	in := new(ContentDraft)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(HostServiceServer).SavePage(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/" + HostServiceName + "/SavePage"}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(HostServiceServer).SavePage(ctx, req.(*ContentDraft))
	}
	return interceptor(ctx, in, info, handler)
}

func hostReloadRuntimeHandler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	in := new(ReloadRuntimeRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(HostServiceServer).ReloadRuntime(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/" + HostServiceName + "/ReloadRuntime"}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(HostServiceServer).ReloadRuntime(ctx, req.(*ReloadRuntimeRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func hostCreateContentExportHandler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	in := new(CreateContentExportRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(HostServiceServer).CreateContentExport(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/" + HostServiceName + "/CreateContentExport"}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(HostServiceServer).CreateContentExport(ctx, req.(*CreateContentExportRequest))
	}
	return interceptor(ctx, in, info, handler)
}
