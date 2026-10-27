package procmon

import (
	"runtime"
	"sort"
	"strings"

	gopsnet "github.com/shirou/gopsutil/v4/net"
)

func runtimeCPUCount() int {
	n := runtime.NumCPU()
	if n < 1 {
		return 1
	}
	return n
}

// analyzeDependencies 基于 TCP 连接关系与监听端口推导进程间依赖，
// 同时聚合出可识别的服务节点（nginx / mysql / redis 等）。
func analyzeDependencies(
	procs []Proc,
	index map[int32]*Proc,
	listeners map[uint32]int32,
	perPID map[int32][]gopsnet.ConnectionStat,
) ([]Edge, []ServiceNode) {

	local := localAddressSet()
	edges := make([]Edge, 0, 64)
	seenEdge := make(map[[2]int32]struct{}, 64)

	// 父子关系边，用于前端还原启动链。
	for i := range procs {
		p := &procs[i]
		if p.PPID <= 0 {
			continue
		}
		if _, ok := index[p.PPID]; !ok {
			continue
		}
		key := [2]int32{p.PID, p.PPID}
		if _, dup := seenEdge[key]; dup {
			continue
		}
		seenEdge[key] = struct{}{}
		edges = append(edges, Edge{From: p.PID, To: p.PPID, Kind: "parent"})
	}

	// 网络依赖：A 建立到本机 B 监听端口的连接，视为 A 依赖 B。
	clients := make(map[int32]map[int32]struct{}, 32)
	for i := range procs {
		p := &procs[i]
		var deps []string
		for _, cn := range perPID[p.PID] {
			if cn.Status != "ESTABLISHED" && cn.Status != "SYN_SENT" {
				continue
			}
			if cn.Raddr.Port == 0 {
				continue
			}
			if _, ok := local[cn.Raddr.IP]; !ok {
				continue
			}
			owner, ok := listeners[cn.Raddr.Port]
			if !ok || owner == p.PID {
				continue
			}
			ownerProc, ok := index[owner]
			if !ok {
				continue
			}
			dep := ownerProc.Service
			if dep == "" {
				dep = fallbackServiceName(ownerProc.Name, ownerProc.Exe)
			}
			deps = append(deps, dep)

			key := [2]int32{p.PID, owner}
			if _, dup := seenEdge[key]; !dup {
				seenEdge[key] = struct{}{}
				edges = append(edges, Edge{From: p.PID, To: owner, Kind: "network"})
			}
			if clients[owner] == nil {
				clients[owner] = make(map[int32]struct{}, 4)
			}
			clients[owner][p.PID] = struct{}{}
		}
		p.Dependencies = uniqueStrings(deps)
	}

	// 聚合服务节点。
	type agg struct {
		name    string
		kind    string
		pids    map[int32]struct{}
		ports   map[uint32]struct{}
		clients map[int32]struct{}
	}
	nodes := make(map[string]*agg, 16)
	for i := range procs {
		p := &procs[i]
		clientSet := clients[p.PID]
		if p.Service == "" && len(clientSet) == 0 {
			continue
		}

		key := "svc:" + strings.ToLower(p.Service)
		name := p.Service
		kind := p.ServiceKind
		if p.Service == "" {
			name = fallbackServiceName(p.Name, p.Exe)
			kind = "其他服务"
			key = "proc:" + strings.ToLower(name)
		}

		a, ok := nodes[key]
		if !ok {
			a = &agg{
				name:    name,
				kind:    kind,
				pids:    make(map[int32]struct{}, 2),
				ports:   make(map[uint32]struct{}, 2),
				clients: make(map[int32]struct{}, 4),
			}
			nodes[key] = a
		}
		a.pids[p.PID] = struct{}{}
		for _, port := range p.Ports {
			a.ports[port] = struct{}{}
		}
		for c := range clientSet {
			a.clients[c] = struct{}{}
		}
	}

	services := make([]ServiceNode, 0, len(nodes))
	for key, a := range nodes {
		pids := make([]int32, 0, len(a.pids))
		for pid := range a.pids {
			pids = append(pids, pid)
		}
		sort.Slice(pids, func(i, j int) bool { return pids[i] < pids[j] })

		ports := make([]uint32, 0, len(a.ports))
		for port := range a.ports {
			ports = append(ports, port)
		}
		ports = sortedUniquePorts(ports)

		managed := false
		for _, pid := range pids {
			if owner, ok := index[pid]; ok && owner.Managed {
				managed = true
				break
			}
		}

		services = append(services, ServiceNode{
			Key:     key,
			Name:    a.name,
			Kind:    a.kind,
			PIDs:    pids,
			Ports:   ports,
			Clients: len(a.clients),
			Managed: managed,
		})
	}
	sort.Slice(services, func(i, j int) bool {
		if services[i].Clients != services[j].Clients {
			return services[i].Clients > services[j].Clients
		}
		return services[i].Name < services[j].Name
	})

	sort.Slice(edges, func(i, j int) bool {
		if edges[i].Kind != edges[j].Kind {
			return edges[i].Kind < edges[j].Kind
		}
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})

	return edges, services
}
