package procmon

import (
	"path/filepath"
	"sort"
	"strings"
)

// signature 描述一类可识别的服务：名称、类别、典型端口与进程名特征。
type signature struct {
	Name    string
	Kind    string
	Ports   []uint32
	Matches []string
}

// 常见服务特征库，按优先级从上到下匹配。
var signatures = []signature{
	{Name: "nginx", Kind: "Web 服务器", Ports: []uint32{80, 443, 8080}, Matches: []string{"nginx"}},
	{Name: "Apache HTTPD", Kind: "Web 服务器", Ports: []uint32{80, 443}, Matches: []string{"httpd", "apache"}},
	{Name: "Caddy", Kind: "Web 服务器", Ports: []uint32{80, 443, 2019}, Matches: []string{"caddy"}},
	{Name: "MySQL", Kind: "数据库", Ports: []uint32{3306}, Matches: []string{"mysqld", "mysql"}},
	{Name: "MariaDB", Kind: "数据库", Ports: []uint32{3306}, Matches: []string{"mariadbd", "mariadb"}},
	{Name: "PostgreSQL", Kind: "数据库", Ports: []uint32{5432}, Matches: []string{"postgres"}},
	{Name: "SQL Server", Kind: "数据库", Ports: []uint32{1433}, Matches: []string{"sqlservr"}},
	{Name: "MongoDB", Kind: "数据库", Ports: []uint32{27017}, Matches: []string{"mongod"}},
	{Name: "Redis", Kind: "缓存", Ports: []uint32{6379}, Matches: []string{"redis"}},
	{Name: "Memcached", Kind: "缓存", Ports: []uint32{11211}, Matches: []string{"memcached"}},
	{Name: "Elasticsearch", Kind: "搜索引擎", Ports: []uint32{9200, 9300}, Matches: []string{"elasticsearch"}},
	{Name: "RabbitMQ", Kind: "消息队列", Ports: []uint32{5672, 15672}, Matches: []string{"rabbitmq", "beam.smp"}},
	{Name: "Kafka", Kind: "消息队列", Ports: []uint32{9092}, Matches: []string{"kafka"}},
	{Name: "NATS", Kind: "消息队列", Ports: []uint32{4222}, Matches: []string{"nats-server"}},
	{Name: "MinIO", Kind: "对象存储", Ports: []uint32{9000}, Matches: []string{"minio"}},
	{Name: "ClickHouse", Kind: "数据库", Ports: []uint32{8123, 9000}, Matches: []string{"clickhouse"}},
	{Name: "Docker", Kind: "容器运行时", Ports: []uint32{2375, 2376}, Matches: []string{"dockerd", "com.docker.backend", "docker-proxy"}},
	{Name: "Consul", Kind: "服务治理", Ports: []uint32{8500, 8600}, Matches: []string{"consul"}},
	{Name: "etcd", Kind: "服务治理", Ports: []uint32{2379, 2380}, Matches: []string{"etcd"}},
	{Name: "Nacos", Kind: "服务治理", Ports: []uint32{8848}, Matches: []string{"nacos"}},
	{Name: "Prometheus", Kind: "监控", Ports: []uint32{9090}, Matches: []string{"prometheus"}},
	{Name: "Grafana", Kind: "监控", Ports: []uint32{3000}, Matches: []string{"grafana"}},
	{Name: "IIS", Kind: "Web 服务器", Ports: []uint32{80, 443}, Matches: []string{"w3wp", "inetinfo"}},
	{Name: "Tomcat", Kind: "应用服务器", Ports: []uint32{8080}, Matches: []string{"tomcat", "catalina"}},
}

// detectService 依据进程名 / 可执行路径 / 监听端口识别服务类型。
func detectService(name, exe string, ports []uint32) (string, string) {
	lowerName := strings.ToLower(name)
	baseExe := strings.ToLower(filepath.Base(exe))

	// 先按名称匹配，避免端口冲突（例如 9000 同时是 MinIO / ClickHouse）。
	for _, sig := range signatures {
		for _, m := range sig.Matches {
			if strings.Contains(lowerName, m) || strings.Contains(baseExe, m) {
				return sig.Name, sig.Kind
			}
		}
	}
	if len(ports) == 0 {
		return "", ""
	}
	for _, sig := range signatures {
		for _, p := range sig.Ports {
			for _, lp := range ports {
				if p == lp {
					return sig.Name, sig.Kind
				}
			}
		}
	}
	return "", ""
}

// fallbackServiceName 在无法识别类型时给出易于阅读的占位名称。
func fallbackServiceName(name, exe string) string {
	if name != "" {
		return name
	}
	if exe != "" {
		return filepath.Base(exe)
	}
	return "未知进程"
}

func sortedUniquePorts(in []uint32) []uint32 {
	seen := make(map[uint32]struct{}, len(in))
	out := make([]uint32, 0, len(in))
	for _, p := range in {
		if p == 0 {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
