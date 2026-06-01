package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"golang.org/x/crypto/bcrypt"

	"github.com/zanel1u/cloud-cli-proxy/internal/controlplane/credgen"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

// seedAdminRepo 是 ensureSeedAdminWithRepo 所需的仓储方法子集。
// *repository.Repository 已实现全部方法，无需额外封装；引入 interface 仅为单测注入 fake。
type seedAdminRepo interface {
	GetUserByLoginIdentifierForAuth(ctx context.Context, identifier string) (repository.User, error)
	GetUser(ctx context.Context, userID string) (repository.User, error)
	CreateUserWithRole(ctx context.Context, p repository.CreateUserWithRoleParams) (repository.User, error)
	UpdateUserPassword(ctx context.Context, userID, passwordHash string) error
	UpdateUserEntryPassword(ctx context.Context, userID, entryPassword string) error
	UpdateUserSSHKeys(ctx context.Context, userID, publicKey, privateKey, keyType string) error
	CreateSSHKey(ctx context.Context, userID, purpose, label, publicKey, privateKey, keyType, fingerprint string) (repository.SSHKey, error)
	ListSSHKeysByUserAndPurpose(ctx context.Context, userID, purpose string) ([]repository.SSHKey, error)
}

// seedAdminFixPlan 描述对一个已存在 seed admin 用户需要执行哪些写动作。
// 三个布尔位互相独立，可同时为 true。
type seedAdminFixPlan struct {
	NeedEntryPassword bool // users.entry_password 为空 → 需写
	NeedSSHKeys       bool // users.ssh_public_key / ssh_private_key 任一为空 → 需写
	NeedSSHKeysRow    bool // ssh_keys 表无 purpose='inbound' label='auto-generated' 行 → 需插
}

// planSeedAdminCredentialFix 是纯函数：根据当前 user 行的凭据列与 ssh_keys 表
// 是否已存在 inbound/auto-generated 行，计算需要执行哪些补齐动作。
//
// 当所有列齐全且 hasInboundAutogen=true 时，返回三个 false（即无操作）。
func planSeedAdminCredentialFix(user repository.User, hasInboundAutogen bool) seedAdminFixPlan {
	return seedAdminFixPlan{
		NeedEntryPassword: user.EntryPassword == "",
		NeedSSHKeys:       user.SSHPublicKey == "" || user.SSHPrivateKey == "",
		NeedSSHKeysRow:    !hasInboundAutogen,
	}
}

// ensureSeedAdminWithRepo 在控制面启动时确保种子 admin 凭据齐全：
//   - 全新部署：CreateUserWithRole + 写 entry_password + 写 ssh_*  + 插 ssh_keys 行
//   - 已存在但缺凭据：仅补齐缺失部分（幂等，不重复插 ssh_keys 行）
//   - 已存在且齐全：完全 no-op
//   - 任何步骤失败 → fail-fast 返回 error，调用方应让控制面启动失败，避免带病启动
//
// username/password 为空时记日志后返回 nil（开发态默认行为，与原实现保持一致）。
func ensureSeedAdminWithRepo(ctx context.Context, logger *slog.Logger, repo seedAdminRepo, username, password string) error {
	if username == "" || password == "" {
		logger.Warn("seed admin: ADMIN_USERNAME or ADMIN_PASSWORD not set, skipping")
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}

	// 提前生成一组凭据：fail-fast 把密钥生成的失败前置到所有写库前。
	entryPwd := credgen.GenerateEntryPassword()
	pub, priv, err := credgen.GenerateSSHKeyPair("ed25519", username)
	if err != nil {
		return fmt.Errorf("generate seed admin ssh key: %w", err)
	}
	fingerprint := credgen.ComputeFingerprint(pub)
	if fingerprint == "" {
		return fmt.Errorf("compute seed admin ssh fingerprint: empty")
	}

	existing, err := repo.GetUserByLoginIdentifierForAuth(ctx, username)
	var user repository.User
	switch {
	case errors.Is(err, sql.ErrNoRows):
		user, err = repo.CreateUserWithRole(ctx, repository.CreateUserWithRoleParams{
			Username:     username,
			PasswordHash: string(hash),
			ShortID:      username,
			Role:         "admin",
		})
		if err != nil {
			return fmt.Errorf("create seed admin: %w", err)
		}
		logger.Info("seed admin created", "short_id", username)
	case err == nil:
		// GetUserByLoginIdentifierForAuth 仅返回基础列（不含 entry_password / ssh_*），
		// 必须再调 GetUser(id) 拿完整凭据列以决定是否需要补齐。
		user, err = repo.GetUser(ctx, existing.ID)
		if err != nil {
			return fmt.Errorf("get seed admin full row: %w", err)
		}

		// 每次启动都校验密码是否与 .env 配置一致，不一致则自动同步。
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
			logger.Info("seed admin password changed in config, syncing to db")
			if err := repo.UpdateUserPassword(ctx, user.ID, string(hash)); err != nil {
				return fmt.Errorf("sync seed admin password: %w", err)
			}
			user.PasswordHash = string(hash)
		}
	default:
		return fmt.Errorf("check seed admin: %w", err)
	}

	keys, err := repo.ListSSHKeysByUserAndPurpose(ctx, user.ID, "inbound")
	if err != nil {
		return fmt.Errorf("list inbound ssh keys: %w", err)
	}
	hasInboundAutogen := false
	for _, k := range keys {
		if k.Label == "auto-generated" {
			hasInboundAutogen = true
			break
		}
	}

	fix := planSeedAdminCredentialFix(user, hasInboundAutogen)

	if fix.NeedEntryPassword {
		if err := repo.UpdateUserEntryPassword(ctx, user.ID, entryPwd); err != nil {
			return fmt.Errorf("backfill seed admin entry password: %w", err)
		}
	}
	if fix.NeedSSHKeys {
		if err := repo.UpdateUserSSHKeys(ctx, user.ID, pub, priv, "ed25519"); err != nil {
			return fmt.Errorf("backfill seed admin ssh keys: %w", err)
		}
	}
	if fix.NeedSSHKeysRow {
		// 仅当 NeedSSHKeys 也为 true 时，新生成的 pub/priv 才与 users 表保持一致；
		// 若 users 表已有密钥（NeedSSHKeys=false）但 ssh_keys 表无 auto-generated 行，
		// 必须复用 users 表里的现有密钥写 ssh_keys，避免两表持有两套不同密钥。
		rowPub, rowPriv, rowFP := pub, priv, fingerprint
		if !fix.NeedSSHKeys {
			rowPub = user.SSHPublicKey
			rowPriv = user.SSHPrivateKey
			rowFP = credgen.ComputeFingerprint(rowPub)
			if rowFP == "" {
				return fmt.Errorf("compute fingerprint from existing seed admin ssh key: empty")
			}
		}
		if _, err := repo.CreateSSHKey(ctx, user.ID, "inbound", "auto-generated", rowPub, rowPriv, "ed25519", rowFP); err != nil {
			return fmt.Errorf("create seed admin ssh_keys row: %w", err)
		}
	}

	if fix.NeedEntryPassword || fix.NeedSSHKeys || fix.NeedSSHKeysRow {
		logger.Info("seed admin credentials backfilled",
			"short_id", username,
			"entry_password", fix.NeedEntryPassword,
			"ssh_keys", fix.NeedSSHKeys,
			"ssh_keys_row", fix.NeedSSHKeysRow,
		)
	}
	return nil
}
