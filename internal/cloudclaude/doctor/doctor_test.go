package doctor

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude"
)

// TestRunDoctor_NoInit_NetworkAuthOnlyLocal — CONTEXT D-06：未 init 时 mount/ssh/disk 仍跑（输出 Skip 即可）。
func TestRunDoctor_NoInit_NetworkAuthOnlyLocal(t *testing.T) {
	origCfg := loadConfig
	loadConfig = func() (*cloudclaude.Config, error) { return nil, fmt.Errorf("config 不存在") }
	t.Cleanup(func() { loadConfig = origCfg })
	origLH := lookupHost
	lookupHost = func(host string) ([]string, error) { return []string{"127.0.0.1"}, nil }
	t.Cleanup(func() { lookupHost = origLH })

	r, err := RunDoctor(context.Background(), Options{Domain: "all", CheckTimeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("RunDoctor err: %v", err)
	}
	if r.SchemaVersion != 1 {
		t.Errorf("SchemaVersion 必须为 1，实际 %d", r.SchemaVersion)
	}
	foundDomains := map[string]int{}
	for _, c := range r.Checks {
		foundDomains[c.Domain]++
	}
	if foundDomains["network"] == 0 {
		t.Error("network 维度未 run")
	}
	if foundDomains["auth"] == 0 {
		t.Error("auth 维度未 run")
	}
}

// TestRunDoctor_DomainFilter — --domain=network 只跑 network 维度。
func TestRunDoctor_DomainFilter(t *testing.T) {
	origCfg := loadConfig
	loadConfig = func() (*cloudclaude.Config, error) { return nil, fmt.Errorf("config 不存在") }
	t.Cleanup(func() { loadConfig = origCfg })
	origLH := lookupHost
	lookupHost = func(host string) ([]string, error) { return nil, fmt.Errorf("no host") }
	t.Cleanup(func() { lookupHost = origLH })

	r, err := RunDoctor(context.Background(), Options{Domain: "network", CheckTimeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("RunDoctor err: %v", err)
	}
	for _, c := range r.Checks {
		if c.Domain != "network" {
			t.Errorf("Domain filter 失效：出现 %s.%s", c.Domain, c.Name)
		}
	}
}

// TestRunDoctor_SummaryAggregation — Summary 计数正确。
func TestRunDoctor_SummaryAggregation(t *testing.T) {
	r := &Report{
		Checks: []Check{
			{Status: StatusPass}, {Status: StatusPass},
			{Status: StatusWarn}, {Status: StatusFail},
			{Status: StatusSkip}, {Status: StatusSkip}, {Status: StatusSkip},
		},
	}
	for _, c := range r.Checks {
		r.Summary.Total++
		switch c.Status {
		case StatusPass:
			r.Summary.Pass++
		case StatusWarn:
			r.Summary.Warn++
		case StatusFail:
			r.Summary.Fail++
		case StatusSkip:
			r.Summary.Skip++
		}
	}
	if r.Summary.Total != 7 || r.Summary.Pass != 2 || r.Summary.Warn != 1 ||
		r.Summary.Fail != 1 || r.Summary.Skip != 3 {
		t.Errorf("聚合错误：%+v", r.Summary)
	}
}
