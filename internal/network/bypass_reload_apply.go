package network

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// 容器内白名单文件存放目录。sing-box rule_set 引用此路径。
const bypassContainerDir = "/etc/cloud-claude/bypass"

// 测试注入点：默认绑定真实 docker exec 实现，单测替换为 fake 闭包。
var (
	dockerExecNftHook   = dockerExecNft
	dockerWriteFileHook = dockerWriteFileInContainer
	dockerReadFileHook  = dockerReadFileInContainer
)

// ApplyBypassRuleSet 把 snapshot 的 cidrsJSON / domainsJSON 落盘 +
// 同步更新 nft @whitelist_v4 set。
//
// 跨平台实现（docker exec）。
// sing-box 通过 rule_set (type=local) 引用白名单文件，文件 watcher
// 检测变化后自动热加载，无需 SIGHUP。
//
// 严格顺序（任意一步失败立即返回 error）：
//  1. 解析 cidrsJSON → []string
//  2. 拼 nft 事务脚本
//  3. 通过 docker exec 执行 nft -f 脚本
//  4. 通过 docker exec 写白名单 JSON 文件（sing-box rule_set 自动 reload）
func ApplyBypassRuleSet(ctx context.Context, hostID string, cidrsJSON, domainsJSON json.RawMessage) error {
	cidrs, err := extractCIDRsFromRuleSetJSON(cidrsJSON)
	if err != nil {
		return fmt.Errorf("parse cidrs json: %w", err)
	}

	script := buildNftWhitelistUpdateScript(cidrs)

	if err := dockerExecNftHook(ctx, hostID, script); err != nil {
		return err
	}

	if err := dockerWriteFileHook(ctx, hostID, "whitelist-cidrs.json", cidrsJSON); err != nil {
		return fmt.Errorf("write whitelist-cidrs.json: %w", err)
	}
	if err := dockerWriteFileHook(ctx, hostID, "whitelist-domains.json", domainsJSON); err != nil {
		return fmt.Errorf("write whitelist-domains.json: %w", err)
	}

	return nil
}

// VerifyBypassConsistency 读取容器内 whitelist-cidrs.json 与
// nft -j list set ip cloudproxy whitelist_v4 输出，
// 分别按 CIDR 集合归一化 sha256 计算 hash 并比较。
func VerifyBypassConsistency(ctx context.Context, hostID string) (ConsistencyResult, error) {
	// 1. 文件侧
	fileBytes, err := dockerReadFileHook(ctx, hostID, "whitelist-cidrs.json")
	if err != nil {
		return ConsistencyResult{}, fmt.Errorf("read whitelist-cidrs.json: %w", err)
	}
	fileCIDRs, err := extractCIDRsFromRuleSetJSON(fileBytes)
	if err != nil {
		return ConsistencyResult{}, fmt.Errorf("parse whitelist-cidrs.json: %w", err)
	}
	fileHash := normalizedSHA256(fileCIDRs)

	// 2. nft set 侧
	nftOut, err := dockerListNftSetHook(ctx, hostID)
	if err != nil {
		return ConsistencyResult{}, fmt.Errorf("nft list set: %w", err)
	}
	nftCIDRs, err := extractCIDRsFromNftJSON(nftOut)
	if err != nil {
		return ConsistencyResult{}, fmt.Errorf("parse nft output: %w", err)
	}
	nftHash := normalizedSHA256(nftCIDRs)

	res := ConsistencyResult{
		RuleSetSHA256: fileHash,
		NftSetSHA256:  nftHash,
		OK:            fileHash == nftHash,
	}
	if !res.OK {
		res.Detail = fmt.Sprintf("cidr set mismatch: file=%d entries, nft=%d entries", len(fileCIDRs), len(nftCIDRs))
	}
	return res, nil
}

// 测试注入点：dockerListNftSetHook 默认调 docker exec。
var dockerListNftSetHook = dockerListNftSet

// ---------- 默认实现（docker exec） ----------

func containerName(hostID string) string {
	return "cloudproxy-" + hostID
}

// dockerExecNft 将 nft 脚本写入容器临时文件并执行。
func dockerExecNft(ctx context.Context, hostID, nftScript string) error {
	name := containerName(hostID)
	script := fmt.Sprintf(
		`mkdir -p %s && cat > /tmp/bypass-reload.nft && nft -f /tmp/bypass-reload.nft && rm -f /tmp/bypass-reload.nft`,
		bypassContainerDir,
	)
	cmd := dockerExecContext(ctx, name, "sh", "-c", script)
	cmd.Stdin = strings.NewReader(nftScript)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker exec nft: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// dockerWriteFileInContainer 将内容写入容器内指定文件。
func dockerWriteFileInContainer(ctx context.Context, hostID, filename string, data []byte) error {
	name := containerName(hostID)
	path := bypassContainerDir + "/" + filename
	script := fmt.Sprintf(`mkdir -p %s && cat > %s`, bypassContainerDir, path)
	cmd := dockerExecContext(ctx, name, "sh", "-c", script)
	cmd.Stdin = strings.NewReader(string(data))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker exec write %s: %s: %w", filename, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// dockerReadFileInContainer 读取容器内指定文件。
func dockerReadFileInContainer(ctx context.Context, hostID, filename string) ([]byte, error) {
	name := containerName(hostID)
	path := bypassContainerDir + "/" + filename
	cmd := dockerExecContext(ctx, name, "cat", path)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker exec cat %s: %w", path, err)
	}
	return out, nil
}

// dockerListNftSet 通过 docker exec 获取 nft set 的 JSON 输出。
func dockerListNftSet(ctx context.Context, hostID string) ([]byte, error) {
	name := containerName(hostID)
	cmd := dockerExecContext(ctx, name, "nft", "-j", "list", "set", bypassNftFamily, bypassNftTable, bypassNftSetName)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker exec nft list set: %w", err)
	}
	return out, nil
}
