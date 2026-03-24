package gateway

import (
	"context"
	"encoding/json"
	"time"

	vmsanv1 "github.com/angelorc/vmsan/hostd/gen/vmsan/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Ensure Server implements the gRPC GatewayService interface.
var _ vmsanv1.GatewayServiceServer = (*Server)(nil)

// ---------------------------------------------------------------------------
// System RPCs
// ---------------------------------------------------------------------------

func (s *Server) Ping(_ context.Context, _ *vmsanv1.PingRequest) (*vmsanv1.PingResponse, error) {
	return &vmsanv1.PingResponse{
		Version: Version,
		Vms:     int32(len(s.manager.ListVMs())),
	}, nil
}

func (s *Server) Health(_ context.Context, _ *vmsanv1.HealthRequest) (*vmsanv1.HealthResponse, error) {
	vms := s.manager.ListVMs()
	return &vmsanv1.HealthResponse{
		Version:    Version,
		Vms:        int32(len(vms)),
		Uptime:     time.Since(s.startTime).Truncate(time.Second).String(),
		DnsProxies: int32(s.dnsSupervisor.Count()),
		SniProxies: int32(len(vms)),
	}, nil
}

func (s *Server) Status(_ context.Context, _ *vmsanv1.StatusRequest) (*vmsanv1.StatusResponse, error) {
	metas, err := listVMMetadata()
	if err != nil {
		// Fall back to manager state count.
		vms := s.manager.ListVMs()
		return &vmsanv1.StatusResponse{Vms: int32(len(vms))}, nil
	}
	pbList := make([]*vmsanv1.VMMetadata, len(metas))
	for i, m := range metas {
		pbList[i] = vmMetadataToProto(m)
	}
	return &vmsanv1.StatusResponse{
		Vms:  int32(len(metas)),
		List: pbList,
	}, nil
}

func (s *Server) Shutdown(_ context.Context, _ *vmsanv1.ShutdownRequest) (*vmsanv1.ShutdownResponse, error) {
	s.timeoutManager.CancelAll()
	s.manager.StopAll()
	if s.cancelFunc != nil {
		s.cancelFunc()
	}
	return &vmsanv1.ShutdownResponse{}, nil
}

func (s *Server) Doctor(_ context.Context, _ *vmsanv1.DoctorRequest) (*vmsanv1.DoctorResponse, error) {
	checks := runDoctorChecks()
	pbChecks := make([]*vmsanv1.DoctorCheck, len(checks))
	for i, c := range checks {
		pbChecks[i] = &vmsanv1.DoctorCheck{
			Category: c.Category,
			Name:     c.Name,
			Status:   c.Status,
			Detail:   c.Detail,
			Fix:      c.Fix,
		}
	}
	return &vmsanv1.DoctorResponse{Checks: pbChecks}, nil
}

// ---------------------------------------------------------------------------
// VM lifecycle
// ---------------------------------------------------------------------------

func (s *Server) CreateVM(ctx context.Context, req *vmsanv1.CreateVMRequest) (*vmsanv1.CreateVMResponse, error) {
	p := vmCreateParams{
		VCPUs:          int(req.Vcpus),
		MemMiB:         int(req.MemMib),
		Runtime:        req.Runtime,
		DiskSizeGb:     req.DiskSizeGb,
		NetworkPolicy:  req.NetworkPolicy,
		Domains:        req.Domains,
		AllowedCIDRs:   req.AllowedCidrs,
		DeniedCIDRs:    req.DeniedCidrs,
		Ports:          toIntSlice(req.Ports),
		BandwidthMbit:  int(req.BandwidthMbit),
		AllowICMP:      req.AllowIcmp,
		Project:        req.Project,
		Service:        req.Service,
		ConnectTo:      req.ConnectTo,
		SkipDNAT:       req.SkipDnat,
		KernelPath:     req.KernelPath,
		RootfsPath:     req.RootfsPath,
		SnapshotID:     req.SnapshotId,
		AgentBinary:    req.AgentBinary,
		AgentToken:     req.AgentToken,
		VMId:           req.VmId,
		DisableSeccomp: req.DisableSeccomp,
		DisablePidNs:   req.DisablePidNs,
		DisableCgroup:  req.DisableCgroup,
		SeccompFilter:  req.SeccompFilter,
		JailerBaseDir:  req.JailerBaseDir,
		OwnerUID:       int(req.OwnerUid),
		OwnerGID:       int(req.OwnerGid),
		TimeoutMs:      req.TimeoutMs,
		TimeoutAt:      req.TimeoutAt,
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal params: %s", err)
	}
	resp := s.handleVMCreateImpl(ctx, raw)
	if !resp.OK {
		return nil, responseToGRPCError(resp)
	}
	cr, err := extractCreateResponse(resp.VM)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "extract response: %s", err)
	}
	return cr, nil
}

func (s *Server) DeleteVM(ctx context.Context, req *vmsanv1.DeleteVMRequest) (*vmsanv1.DeleteVMResponse, error) {
	p := vmDeleteParams{
		VMId:          req.VmId,
		Force:         req.Force,
		JailerBaseDir: req.JailerBaseDir,
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal params: %s", err)
	}
	resp := s.handleVMDeleteImpl(ctx, raw)
	if !resp.OK {
		return nil, responseToGRPCError(resp)
	}
	return &vmsanv1.DeleteVMResponse{}, nil
}

func (s *Server) RestartVM(ctx context.Context, req *vmsanv1.RestartVMRequest) (*vmsanv1.RestartVMResponse, error) {
	p := vmRestartParams{
		VMId:           req.VmId,
		Slot:           int(req.Slot),
		ChrootDir:      req.ChrootDir,
		SocketPath:     req.SocketPath,
		NetworkPolicy:  req.NetworkPolicy,
		Domains:        req.Domains,
		AllowedCIDRs:   req.AllowedCidrs,
		DeniedCIDRs:    req.DeniedCidrs,
		Ports:          toIntSlice(req.Ports),
		BandwidthMbit:  int(req.BandwidthMbit),
		AllowICMP:      req.AllowIcmp,
		SkipDNAT:       req.SkipDnat,
		Project:        req.Project,
		Service:        req.Service,
		ConnectTo:      req.ConnectTo,
		DisableSeccomp: req.DisableSeccomp,
		DisablePidNs:   req.DisablePidNs,
		DisableCgroup:  req.DisableCgroup,
		SeccompFilter:  req.SeccompFilter,
		VCPUs:          int(req.Vcpus),
		MemMiB:         int(req.MemMib),
		KernelPath:     req.KernelPath,
		RootfsPath:     req.RootfsPath,
		AgentBinary:    req.AgentBinary,
		AgentToken:     req.AgentToken,
		NetNSName:      req.NetNsName,
		JailerBaseDir:  req.JailerBaseDir,
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal params: %s", err)
	}
	resp := s.handleVMRestart(ctx, raw)
	if !resp.OK {
		return nil, responseToGRPCError(resp)
	}
	cr, err := extractCreateResponse(resp.VM)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "extract response: %s", err)
	}
	return &vmsanv1.RestartVMResponse{
		VmId:       cr.VmId,
		Slot:       cr.Slot,
		HostIp:     cr.HostIp,
		GuestIp:    cr.GuestIp,
		MeshIp:     cr.MeshIp,
		TapDevice:  cr.TapDevice,
		MacAddress: cr.MacAddress,
		NetNsName:  cr.NetNsName,
		VethHost:   cr.VethHost,
		VethGuest:  cr.VethGuest,
		SubnetMask: cr.SubnetMask,
		ChrootDir:  cr.ChrootDir,
		SocketPath: cr.SocketPath,
		Pid:        cr.Pid,
		AgentToken: cr.AgentToken,
		DnsPort:    cr.DnsPort,
		SniPort:    cr.SniPort,
		HttpPort:   cr.HttpPort,
	}, nil
}

func (s *Server) StartVM(_ context.Context, req *vmsanv1.StartVMRequest) (*vmsanv1.StartVMResponse, error) {
	p := vmStartParams{
		VMId:           req.VmId,
		Slot:           int(req.Slot),
		Policy:         req.Policy,
		AllowedDomains: req.AllowedDomains,
		Project:        req.Project,
		Service:        req.Service,
		ConnectTo:      req.ConnectTo,
		VethHost:       req.VethHost,
		NetNS:          req.NetNs,
		GuestDev:       req.GuestDev,
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal params: %s", err)
	}
	resp := s.handleVMStart(raw)
	if !resp.OK {
		return nil, responseToGRPCError(resp)
	}
	// The VM field is a vmStartResponse (embedded *VMState + mesh fields).
	data, err := json.Marshal(resp.VM)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal vm response: %s", err)
	}
	var startResp struct {
		VMId    string `json:"vmId"`
		Policy  string `json:"policy"`
		MeshIP  string `json:"meshIp"`
		Service string `json:"meshService"`
	}
	if err := json.Unmarshal(data, &startResp); err != nil {
		return nil, status.Errorf(codes.Internal, "unmarshal vm response: %s", err)
	}
	return &vmsanv1.StartVMResponse{
		VmId:        startResp.VMId,
		Policy:      startResp.Policy,
		MeshIp:      startResp.MeshIP,
		MeshService: startResp.Service,
	}, nil
}

func (s *Server) StopVM(_ context.Context, req *vmsanv1.StopVMRequest) (*vmsanv1.StopVMResponse, error) {
	p := vmStopParams{
		VMId:     req.VmId,
		VethHost: req.VethHost,
		NetNS:    req.NetNs,
		GuestDev: req.GuestDev,
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal params: %s", err)
	}
	resp := s.handleVMStop(raw)
	if !resp.OK {
		return nil, responseToGRPCError(resp)
	}
	return &vmsanv1.StopVMResponse{}, nil
}

func (s *Server) FullStopVM(ctx context.Context, req *vmsanv1.FullStopVMRequest) (*vmsanv1.FullStopVMResponse, error) {
	p := vmFullStopParams{
		VMId:          req.VmId,
		Slot:          int(req.Slot),
		PID:           int(req.Pid),
		NetNSName:     req.NetNsName,
		SocketPath:    req.SocketPath,
		JailerBaseDir: req.JailerBaseDir,
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal params: %s", err)
	}
	resp := s.handleVMFullStop(ctx, raw)
	if !resp.OK {
		return nil, responseToGRPCError(resp)
	}
	return &vmsanv1.FullStopVMResponse{}, nil
}

func (s *Server) GetVM(_ context.Context, req *vmsanv1.GetVMRequest) (*vmsanv1.GetVMResponse, error) {
	if req.VmId == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}
	meta, err := readVMMetadata(req.VmId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "VM not found: %s", req.VmId)
	}
	return &vmsanv1.GetVMResponse{Vm: vmMetadataToProto(meta)}, nil
}

func (s *Server) ExtendTimeout(_ context.Context, req *vmsanv1.ExtendTimeoutRequest) (*vmsanv1.ExtendTimeoutResponse, error) {
	p := extendTimeoutParams{
		VMId:      req.VmId,
		TimeoutAt: req.TimeoutAt,
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal params: %s", err)
	}
	resp := s.handleExtendTimeout(raw)
	if !resp.OK {
		return nil, responseToGRPCError(resp)
	}
	return &vmsanv1.ExtendTimeoutResponse{}, nil
}

// ---------------------------------------------------------------------------
// Network policy
// ---------------------------------------------------------------------------

func (s *Server) UpdatePolicy(_ context.Context, req *vmsanv1.UpdatePolicyRequest) (*vmsanv1.UpdatePolicyResponse, error) {
	p := vmUpdatePolicyParams{
		VMId:   req.VmId,
		Policy: req.Policy,
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal params: %s", err)
	}
	resp := s.handleVMUpdatePolicy(raw)
	if !resp.OK {
		return nil, responseToGRPCError(resp)
	}
	return &vmsanv1.UpdatePolicyResponse{}, nil
}

func (s *Server) FullUpdatePolicy(ctx context.Context, req *vmsanv1.FullUpdatePolicyRequest) (*vmsanv1.FullUpdatePolicyResponse, error) {
	p := vmFullUpdatePolicyParams{
		VMId:         req.VmId,
		Policy:       req.Policy,
		Slot:         int(req.Slot),
		Domains:      req.Domains,
		AllowedCIDRs: req.AllowedCidrs,
		DeniedCIDRs:  req.DeniedCidrs,
		Ports:        toIntSlice(req.Ports),
		AllowICMP:    req.AllowIcmp,
		SkipDNAT:     req.SkipDnat,
		NetNSName:    req.NetNsName,
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal params: %s", err)
	}
	resp := s.handleVMFullUpdatePolicy(ctx, raw)
	if !resp.OK {
		return nil, responseToGRPCError(resp)
	}
	return &vmsanv1.FullUpdatePolicyResponse{}, nil
}

// ---------------------------------------------------------------------------
// Snapshots
// ---------------------------------------------------------------------------

func (s *Server) CreateSnapshot(ctx context.Context, req *vmsanv1.CreateSnapshotRequest) (*vmsanv1.CreateSnapshotResponse, error) {
	p := vmSnapshotCreateParams{
		VMId:          req.VmId,
		SnapshotID:    req.SnapshotId,
		SocketPath:    req.SocketPath,
		DestDir:       req.DestDir,
		ChrootDir:     req.ChrootDir,
		JailerBaseDir: req.JailerBaseDir,
		OwnerUID:      int(req.OwnerUid),
		OwnerGID:      int(req.OwnerGid),
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal params: %s", err)
	}
	resp := s.handleVMSnapshotCreate(ctx, raw)
	if !resp.OK {
		return nil, responseToGRPCError(resp)
	}
	return &vmsanv1.CreateSnapshotResponse{
		VmId:       req.VmId,
		SnapshotId: req.SnapshotId,
		DestDir:    req.DestDir,
	}, nil
}

// ---------------------------------------------------------------------------
// Network setup/teardown
// ---------------------------------------------------------------------------

func (s *Server) SetupNetwork(ctx context.Context, req *vmsanv1.SetupNetworkRequest) (*vmsanv1.SetupNetworkResponse, error) {
	p := networkSetupParams{
		VMId:          req.VmId,
		Slot:          int(req.Slot),
		Policy:        req.Policy,
		BandwidthMbit: int(req.BandwidthMbit),
		AllowICMP:     req.AllowIcmp,
		SkipDNAT:      req.SkipDnat,
		AllowedCIDRs:  req.AllowedCidrs,
		DeniedCIDRs:   req.DeniedCidrs,
		Ports:         toIntSlice(req.Ports),
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal params: %s", err)
	}
	resp := s.handleNetworkSetupImpl(ctx, raw)
	if !resp.OK {
		return nil, responseToGRPCError(resp)
	}
	return &vmsanv1.SetupNetworkResponse{}, nil
}

func (s *Server) TeardownNetwork(ctx context.Context, req *vmsanv1.TeardownNetworkRequest) (*vmsanv1.TeardownNetworkResponse, error) {
	p := networkTeardownParams{
		VMId: req.VmId,
		Slot: int(req.Slot),
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal params: %s", err)
	}
	resp := s.handleNetworkTeardownImpl(ctx, raw)
	if !resp.OK {
		return nil, responseToGRPCError(resp)
	}
	return &vmsanv1.TeardownNetworkResponse{}, nil
}

// ---------------------------------------------------------------------------
// Rootfs
// ---------------------------------------------------------------------------

func (s *Server) BuildRootfs(ctx context.Context, req *vmsanv1.BuildRootfsRequest) (*vmsanv1.BuildRootfsResponse, error) {
	p := rootfsBuildParams{
		ImageRef:  req.ImageRef,
		OutputDir: req.OutputDir,
		OwnerUID:  int(req.OwnerUid),
		OwnerGID:  int(req.OwnerGid),
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal params: %s", err)
	}
	resp := s.handleRootfsBuild(ctx, raw)
	if !resp.OK {
		return nil, responseToGRPCError(resp)
	}
	// Extract rootfsPath from response.
	data, _ := json.Marshal(resp.VM)
	var result struct {
		RootfsPath string `json:"rootfsPath"`
		ImageRef   string `json:"imageRef"`
	}
	json.Unmarshal(data, &result)
	return &vmsanv1.BuildRootfsResponse{
		RootfsPath: result.RootfsPath,
		ImageRef:   result.ImageRef,
	}, nil
}

func (s *Server) DownloadRootfs(ctx context.Context, req *vmsanv1.DownloadRootfsRequest) (*vmsanv1.DownloadRootfsResponse, error) {
	p := rootfsDownloadParams{
		URL:      req.Url,
		Checksum: req.Checksum,
		DestPath: req.DestPath,
		OwnerUID: int(req.OwnerUid),
		OwnerGID: int(req.OwnerGid),
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal params: %s", err)
	}
	resp := s.handleRootfsDownload(ctx, raw)
	if !resp.OK {
		return nil, responseToGRPCError(resp)
	}
	// Extract fields from response.
	data, _ := json.Marshal(resp.VM)
	var result struct {
		DestPath string `json:"destPath"`
		Checksum string `json:"checksum"`
		Size     int64  `json:"size"`
	}
	json.Unmarshal(data, &result)
	return &vmsanv1.DownloadRootfsResponse{
		DestPath: result.DestPath,
		Checksum: result.Checksum,
		Size:     result.Size,
	}, nil
}

// ---------------------------------------------------------------------------
// Cloudflare
// ---------------------------------------------------------------------------

func (s *Server) CloudflareSetup(_ context.Context, req *vmsanv1.CloudflareSetupRequest) (*vmsanv1.CloudflareSetupResponse, error) {
	p := cfSetupParams{
		TunnelToken: req.TunnelToken,
		ConfigPath:  req.ConfigPath,
		LogPath:     req.LogPath,
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal params: %s", err)
	}
	resp := s.handleCloudflareSetup(raw)
	if !resp.OK {
		return nil, responseToGRPCError(resp)
	}
	data, _ := json.Marshal(resp.VM)
	var result struct {
		PID int `json:"pid"`
	}
	json.Unmarshal(data, &result)
	return &vmsanv1.CloudflareSetupResponse{Pid: int32(result.PID)}, nil
}

func (s *Server) CloudflareAddRoute(_ context.Context, req *vmsanv1.CloudflareAddRouteRequest) (*vmsanv1.CloudflareAddRouteResponse, error) {
	p := cfAddRouteParams{
		VMId:      req.VmId,
		Hostname:  req.Hostname,
		ApiToken:  req.ApiToken,
		TunnelId:  req.TunnelId,
		AccountId: req.AccountId,
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal params: %s", err)
	}
	resp := s.handleCloudflareAddRoute(raw)
	if !resp.OK {
		return nil, responseToGRPCError(resp)
	}
	return &vmsanv1.CloudflareAddRouteResponse{}, nil
}

func (s *Server) CloudflareRemoveRoute(_ context.Context, req *vmsanv1.CloudflareRemoveRouteRequest) (*vmsanv1.CloudflareRemoveRouteResponse, error) {
	p := cfRemoveRouteParams{
		VMId:      req.VmId,
		ApiToken:  req.ApiToken,
		TunnelId:  req.TunnelId,
		AccountId: req.AccountId,
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal params: %s", err)
	}
	resp := s.handleCloudflareRemoveRoute(raw)
	if !resp.OK {
		return nil, responseToGRPCError(resp)
	}
	return &vmsanv1.CloudflareRemoveRouteResponse{}, nil
}

func (s *Server) CloudflareStatus(_ context.Context, _ *vmsanv1.CloudflareStatusRequest) (*vmsanv1.CloudflareStatusResponse, error) {
	resp := s.handleCloudflareStatus()
	if !resp.OK {
		return nil, responseToGRPCError(resp)
	}
	data, _ := json.Marshal(resp.VM)
	var result struct {
		Running bool   `json:"running"`
		PID     int    `json:"pid"`
		Uptime  string `json:"uptime"`
	}
	json.Unmarshal(data, &result)
	return &vmsanv1.CloudflareStatusResponse{
		Running: result.Running,
		Pid:     int32(result.PID),
		Uptime:  result.Uptime,
	}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// vmMetadataToProto converts the internal VMMetadata struct to its proto form.
func vmMetadataToProto(m *VMMetadata) *vmsanv1.VMMetadata {
	return &vmsanv1.VMMetadata{
		VmId:       m.VMId,
		Slot:       int32(m.Slot),
		Status:     m.Status,
		HostIp:     m.HostIP,
		GuestIp:    m.GuestIP,
		MeshIp:     m.MeshIP,
		Pid:        int32(m.PID),
		CreatedAt:  m.CreatedAt,
		TimeoutAt:  m.TimeoutAt,
		AgentToken: m.AgentToken,
		Runtime:    m.Runtime,
		Vcpus:      int32(m.VCPUs),
		MemMib:     int32(m.MemMiB),
		DiskSizeGb: m.DiskSizeGb,
		Project:    m.Project,
		Service:    m.Service,
		Network: &vmsanv1.VMNetworkMeta{
			Policy:       m.Network.Policy,
			Domains:      m.Network.Domains,
			AllowedCidrs: m.Network.AllowedCIDRs,
			DeniedCidrs:  m.Network.DeniedCIDRs,
			Ports:        toInt32Slice(m.Network.Ports),
			BandwidthMbit: int32(m.Network.BandwidthMbit),
			AllowIcmp:    m.Network.AllowICMP,
		},
		ChrootDir:  m.ChrootDir,
		SocketPath: m.SocketPath,
		TapDevice:  m.TAPDevice,
		MacAddress: m.MACAddress,
		NetNsName:  m.NetNSName,
		VethHost:   m.VethHost,
		VethGuest:  m.VethGuest,
		SubnetMask: m.SubnetMask,
		DnsPort:    int32(m.DNSPort),
		SniPort:    int32(m.SNIPort),
		HttpPort:   int32(m.HTTPPort),
	}
}

// responseToGRPCError converts a failed Response to a gRPC status error.
func responseToGRPCError(r Response) error {
	code := codes.Internal
	switch r.Code {
	case "PARSE_ERROR":
		code = codes.InvalidArgument
	case "VALIDATION_ERROR":
		code = codes.InvalidArgument
	case "NOT_FOUND":
		code = codes.NotFound
	case "UNKNOWN_METHOD":
		code = codes.Unimplemented
	}
	return status.Errorf(code, "%s", r.Error)
}

// extractCreateResponse converts the VM field from a Response into a CreateVMResponse proto.
func extractCreateResponse(vm any) (*vmsanv1.CreateVMResponse, error) {
	data, err := json.Marshal(vm)
	if err != nil {
		return nil, err
	}
	var cr vmCreateResponse
	if err := json.Unmarshal(data, &cr); err != nil {
		return nil, err
	}
	return &vmsanv1.CreateVMResponse{
		VmId:       cr.VMId,
		Slot:       int32(cr.Slot),
		HostIp:     cr.HostIP,
		GuestIp:    cr.GuestIP,
		MeshIp:     cr.MeshIP,
		TapDevice:  cr.TAPDevice,
		MacAddress: cr.MACAddress,
		NetNsName:  cr.NetNSName,
		VethHost:   cr.VethHost,
		VethGuest:  cr.VethGuest,
		SubnetMask: cr.SubnetMask,
		ChrootDir:  cr.ChrootDir,
		SocketPath: cr.SocketPath,
		Pid:        int32(cr.PID),
		AgentToken: cr.AgentToken,
		DnsPort:    int32(cr.DNSPort),
		SniPort:    int32(cr.SNIPort),
		HttpPort:   int32(cr.HTTPPort),
	}, nil
}

// toIntSlice converts []int32 to []int.
func toIntSlice(in []int32) []int {
	if in == nil {
		return nil
	}
	out := make([]int, len(in))
	for i, v := range in {
		out[i] = int(v)
	}
	return out
}

// toInt32Slice converts []int to []int32.
func toInt32Slice(in []int) []int32 {
	if in == nil {
		return nil
	}
	out := make([]int32, len(in))
	for i, v := range in {
		out[i] = int32(v)
	}
	return out
}
