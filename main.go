package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type PodInfo struct {
	Name       string
	Namespace  string
	NodeName   string
	Status     string
	Restarts   int32
	Age        string
	IP         string
	PhaseClass string
}

type DashboardData struct {
	Pods       []PodInfo
	TotalPods  int
	RunningPods int
	Timestamp  string
	Filter     string
}

var clientset *kubernetes.Clientset

func getClientset() *kubernetes.Clientset {
	config, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			kubeconfig = clientcmd.RecommendedHomeFile
		}
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			log.Fatalf("failed to build kubeconfig: %v", err)
		}
	}
	cs, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("failed to create clientset: %v", err)
	}
	return cs
}

func getAge(podStartTime *metav1.Time) string {
	if podStartTime == nil {
		return "N/A"
	}
	d := time.Since(podStartTime.Time)
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd%dh", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh%dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}

func getPhaseClass(phase corev1.PodPhase) string {
	switch phase {
	case corev1.PodRunning:
		return "status-running"
	case corev1.PodPending:
		return "status-pending"
	case corev1.PodSucceeded:
		return "status-succeeded"
	case corev1.PodFailed:
		return "status-failed"
	default:
		return "status-unknown"
	}
}

func getPodStatus(pod corev1.Pod) string {
	status := string(pod.Status.Phase)
	for _, c := range pod.Status.ContainerStatuses {
		if c.State.Waiting != nil && c.State.Waiting.Reason != "" {
			status = c.State.Waiting.Reason
		}
		if c.State.Terminated != nil && c.State.Terminated.Reason != "" {
			status = c.State.Terminated.Reason
		}
	}
	return status
}

func fetchPods(filter string) ([]PodInfo, error) {
	pods, err := clientset.CoreV1().Pods("").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	var result []PodInfo
	for _, p := range pods.Items {
		name := p.Name
		namespace := p.Namespace
		node := p.Spec.NodeName
		if node == "" {
			node = "-"
		}
		status := getPodStatus(p)
		age := getAge(p.Status.StartTime)
		ip := p.Status.PodIP
		if ip == "" {
			ip = "-"
		}
		var restarts int32
		for _, c := range p.Status.ContainerStatuses {
			restarts += c.RestartCount
		}

		if filter != "" && !strings.Contains(strings.ToLower(name), strings.ToLower(filter)) && !strings.Contains(strings.ToLower(namespace), strings.ToLower(filter)) {
			continue
		}

		result = append(result, PodInfo{
			Name:        name,
			Namespace:   namespace,
			NodeName:    node,
			Status:      status,
			Restarts:    restarts,
			Age:         age,
			IP:          ip,
			PhaseClass:  getPhaseClass(p.Status.Phase),
		})
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Namespace != result[j].Namespace {
			return result[i].Namespace < result[j].Namespace
		}
		return result[i].Name < result[j].Name
	})

	return result, nil
}

func dashboardHandler(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("filter")
	pods, err := fetchPods(filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	running := 0
	for _, p := range pods {
		if p.Status == "Running" {
			running++
		}
	}

	data := DashboardData{
		Pods:        pods,
		TotalPods:   len(pods),
		RunningPods: running,
		Timestamp:   time.Now().Format("15:04:05"),
		Filter:      filter,
	}

	w.Header().Set("Content-Type", "text/html")
	tmpl.Execute(w, data)
}

func apiHandler(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("filter")
	pods, err := fetchPods(filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	running := 0
	for _, p := range pods {
		if p.Status == "Running" {
			running++
		}
	}

	data := DashboardData{
		Pods:        pods,
		TotalPods:   len(pods),
		RunningPods: running,
		Timestamp:   time.Now().Format("15:04:05"),
		Filter:      filter,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

var tmpl *template.Template

func init() {
	tmpl = template.Must(template.New("dashboard").Parse(dashboardHTML))
}

func main() {
	clientset = getClientset()

	http.HandleFunc("/", dashboardHandler)
	http.HandleFunc("/api", apiHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Kubernetes Dashboard running on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>K8s Dashboard</title>
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #0f172a; color: #e2e8f0; padding: 20px; }
.header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; flex-wrap: wrap; gap: 12px; }
.header h1 { font-size: 1.5rem; color: #38bdf8; }
.stats { display: flex; gap: 16px; font-size: 0.9rem; }
.stat { background: #1e293b; padding: 8px 16px; border-radius: 8px; }
.stat span { font-weight: bold; }
.stat .num-total { color: #94a3b8; }
.stat .num-running { color: #4ade80; }
.search-box { display: flex; gap: 8px; align-items: center; }
.search-box input { background: #1e293b; border: 1px solid #334155; color: #e2e8f0; padding: 8px 12px; border-radius: 6px; font-size: 0.9rem; outline: none; width: 220px; }
.search-box input:focus { border-color: #38bdf8; }
.search-box button { background: #38bdf8; color: #0f172a; border: none; padding: 8px 16px; border-radius: 6px; cursor: pointer; font-weight: bold; font-size: 0.85rem; }
.search-box button:hover { background: #7dd3fc; }
.timestamp { color: #64748b; font-size: 0.8rem; }
table { width: 100%; border-collapse: collapse; background: #1e293b; border-radius: 12px; overflow: hidden; }
th { background: #334155; padding: 12px 16px; text-align: left; font-size: 0.8rem; text-transform: uppercase; letter-spacing: 0.05em; color: #94a3b8; }
td { padding: 10px 16px; border-top: 1px solid #334155; font-size: 0.9rem; }
tr:hover { background: #263548; }
.status-running { color: #4ade80; font-weight: bold; }
.status-pending { color: #fbbf24; font-weight: bold; }
.status-succeeded { color: #94a3b8; }
.status-failed { color: #f87171; font-weight: bold; }
.status-unknown { color: #94a3b8; }
.namespace-badge { background: #334155; padding: 2px 8px; border-radius: 4px; font-size: 0.8rem; color: #94a3b8; }
.restarts-warn { color: #fbbf24; }
@media (max-width: 768px) {
  .header { flex-direction: column; align-items: stretch; }
  .search-box input { width: 100%; }
  table { font-size: 0.8rem; }
  th, td { padding: 8px 10px; }
}
</style>
</head>
<body>
<div class="header">
  <div>
    <h1>Kubernetes Dashboard</h1>
    <div class="timestamp">Last updated: {{.Timestamp}}</div>
  </div>
  <div class="stats">
    <div class="stat">Total: <span class="num-total">{{.TotalPods}}</span></div>
    <div class="stat">Running: <span class="num-running">{{.RunningPods}}</span></div>
  </div>
  <form class="search-box" method="get" action="/">
    <input type="text" name="filter" placeholder="Filter by name or namespace..." value="{{.Filter}}">
    <button type="submit">Search</button>
  </form>
</div>
<table>
<thead>
<tr>
  <th>Name</th>
  <th>Namespace</th>
  <th>Status</th>
  <th>Restarts</th>
  <th>Age</th>
  <th>Node</th>
  <th>IP</th>
</tr>
</thead>
<tbody>
{{range .Pods}}
<tr>
  <td>{{.Name}}</td>
  <td><span class="namespace-badge">{{.Namespace}}</span></td>
  <td><span class="{{.PhaseClass}}">{{.Status}}</span></td>
  <td{{if gt .Restarts 0}} class="restarts-warn"{{end}}>{{.Restarts}}</td>
  <td>{{.Age}}</td>
  <td>{{.NodeName}}</td>
  <td>{{.IP}}</td>
</tr>
{{else}}
<tr><td colspan="7" style="text-align:center;padding:40px;color:#64748b;">No pods found</td></tr>
{{end}}
</tbody>
</table>
<script>
setTimeout(function(){ window.location.reload(); }, 15000);
</script>
</body>
</html>`
