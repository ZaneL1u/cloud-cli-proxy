package app

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/zanel1u/cloud-cli-proxy/internal/controlplane/credgen"
	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

// fakeSeedAdminRepo 实现 seedAdminRepo，记录每个方法调用次数 + 可注入返回值/错误。
type fakeSeedAdminRepo struct {
	// inputs
	getByIdentifierResult repository.User
	getByIdentifierErr    error
	getUserResult         repository.User
	getUserErr            error
	createUserResult      repository.User
	createUserErr         error
	updatePasswordErr     error
	updateEntryErr        error
	updateSSHKeysErr      error
	createSSHKeyErr       error
	listInboundKeys       []repository.SSHKey
	listInboundErr        error

	// counters
	getByIdentifierCalls  int
	getUserCalls          int
	createUserCalls       int
	updatePasswordCalls   int
	updateEntryCalls      int
	updateSSHKeysCalls    int
	createSSHKeyCalls     int
	listInboundCalls      int

	// captured args
	lastCreatedSSHKeyPub  string
	lastCreatedSSHKeyPriv string
}

func (r *fakeSeedAdminRepo) GetUserByLoginIdentifierForAuth(ctx context.Context, identifier string) (repository.User, error) {
	r.getByIdentifierCalls++
	return r.getByIdentifierResult, r.getByIdentifierErr
}

func (r *fakeSeedAdminRepo) GetUser(ctx context.Context, userID string) (repository.User, error) {
	r.getUserCalls++
	return r.getUserResult, r.getUserErr
}

func (r *fakeSeedAdminRepo) CreateUserWithRole(ctx context.Context, p repository.CreateUserWithRoleParams) (repository.User, error) {
	r.createUserCalls++
	if r.createUserErr != nil {
		return repository.User{}, r.createUserErr
	}
	if r.createUserResult.ID == "" {
		// 默认填充一个合理 ID，避免后续 GetUser 调用时 user.ID 为空
		return repository.User{ID: "u-new", Username: p.Username, ShortID: p.ShortID, Role: p.Role}, nil
	}
	return r.createUserResult, nil
}

func (r *fakeSeedAdminRepo) UpdateUserPassword(ctx context.Context, userID, passwordHash string) error {
	r.updatePasswordCalls++
	return r.updatePasswordErr
}

func (r *fakeSeedAdminRepo) UpdateUserEntryPassword(ctx context.Context, userID, entryPassword string) error {
	r.updateEntryCalls++
	return r.updateEntryErr
}

func (r *fakeSeedAdminRepo) UpdateUserSSHKeys(ctx context.Context, userID, publicKey, privateKey, keyType string) error {
	r.updateSSHKeysCalls++
	return r.updateSSHKeysErr
}

func (r *fakeSeedAdminRepo) CreateSSHKey(ctx context.Context, userID, purpose, label, publicKey, privateKey, keyType, fingerprint string) (repository.SSHKey, error) {
	r.createSSHKeyCalls++
	r.lastCreatedSSHKeyPub = publicKey
	r.lastCreatedSSHKeyPriv = privateKey
	if r.createSSHKeyErr != nil {
		return repository.SSHKey{}, r.createSSHKeyErr
	}
	return repository.SSHKey{ID: "k-new", UserID: userID, Purpose: purpose, Label: label}, nil
}

func (r *fakeSeedAdminRepo) ListSSHKeysByUserAndPurpose(ctx context.Context, userID, purpose string) ([]repository.SSHKey, error) {
	r.listInboundCalls++
	return r.listInboundKeys, r.listInboundErr
}

func newDiscardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestPlanSeedAdminCredentialFix(t *testing.T) {
	tests := []struct {
		name              string
		user              repository.User
		hasInboundAutogen bool
		want              seedAdminFixPlan
	}{
		{
			name: "齐全无操作",
			user: repository.User{
				EntryPassword: "x", SSHPublicKey: "p", SSHPrivateKey: "k",
			},
			hasInboundAutogen: true,
			want:              seedAdminFixPlan{NeedEntryPassword: false, NeedSSHKeys: false, NeedSSHKeysRow: false},
		},
		{
			name: "缺 entry_password 单独触发",
			user: repository.User{
				EntryPassword: "", SSHPublicKey: "p", SSHPrivateKey: "k",
			},
			hasInboundAutogen: true,
			want:              seedAdminFixPlan{NeedEntryPassword: true, NeedSSHKeys: false, NeedSSHKeysRow: false},
		},
		{
			name: "缺 ssh_public_key 单独触发 NeedSSHKeys",
			user: repository.User{
				EntryPassword: "x", SSHPublicKey: "", SSHPrivateKey: "k",
			},
			hasInboundAutogen: true,
			want:              seedAdminFixPlan{NeedEntryPassword: false, NeedSSHKeys: true, NeedSSHKeysRow: false},
		},
		{
			name: "缺 ssh_keys 行单独触发",
			user: repository.User{
				EntryPassword: "x", SSHPublicKey: "p", SSHPrivateKey: "k",
			},
			hasInboundAutogen: false,
			want:              seedAdminFixPlan{NeedEntryPassword: false, NeedSSHKeys: false, NeedSSHKeysRow: true},
		},
		{
			name:              "全空时三 bool 同时 true",
			user:              repository.User{},
			hasInboundAutogen: false,
			want:              seedAdminFixPlan{NeedEntryPassword: true, NeedSSHKeys: true, NeedSSHKeysRow: true},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := planSeedAdminCredentialFix(tc.user, tc.hasInboundAutogen)
			if got != tc.want {
				t.Fatalf("planSeedAdminCredentialFix = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestEnsureSeedAdmin_FreshInstall(t *testing.T) {
	repo := &fakeSeedAdminRepo{
		getByIdentifierErr: sql.ErrNoRows,
		listInboundKeys:    nil, // 新建用户必然 ssh_keys 表无 auto-generated 行
	}
	if err := ensureSeedAdminWithRepo(context.Background(), newDiscardLogger(), repo, "admin", "p4ss"); err != nil {
		t.Fatalf("ensureSeedAdminWithRepo error = %v", err)
	}
	if repo.createUserCalls != 1 {
		t.Errorf("CreateUserWithRole calls = %d, want 1", repo.createUserCalls)
	}
	if repo.getUserCalls != 0 {
		t.Errorf("GetUser calls = %d, want 0 (fresh install path)", repo.getUserCalls)
	}
	if repo.updateEntryCalls != 1 {
		t.Errorf("UpdateUserEntryPassword calls = %d, want 1", repo.updateEntryCalls)
	}
	if repo.updateSSHKeysCalls != 1 {
		t.Errorf("UpdateUserSSHKeys calls = %d, want 1", repo.updateSSHKeysCalls)
	}
	if repo.createSSHKeyCalls != 1 {
		t.Errorf("CreateSSHKey calls = %d, want 1", repo.createSSHKeyCalls)
	}
}

func TestEnsureSeedAdmin_BackfillExistingUser(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("p4ss"), bcrypt.DefaultCost)
	repo := &fakeSeedAdminRepo{
		getByIdentifierResult: repository.User{ID: "u-1", Username: "admin"},
		getByIdentifierErr:    nil,
		getUserResult: repository.User{
			ID: "u-1", Username: "admin", PasswordHash: string(hash),
			EntryPassword: "", SSHPublicKey: "", SSHPrivateKey: "",
		},
		listInboundKeys: nil,
	}
	if err := ensureSeedAdminWithRepo(context.Background(), newDiscardLogger(), repo, "admin", "p4ss"); err != nil {
		t.Fatalf("ensureSeedAdminWithRepo error = %v", err)
	}
	if repo.createUserCalls != 0 {
		t.Errorf("CreateUserWithRole calls = %d, want 0 (existing user path)", repo.createUserCalls)
	}
	if repo.getUserCalls != 1 {
		t.Errorf("GetUser calls = %d, want 1", repo.getUserCalls)
	}
	if repo.updateEntryCalls != 1 {
		t.Errorf("UpdateUserEntryPassword calls = %d, want 1", repo.updateEntryCalls)
	}
	if repo.updateSSHKeysCalls != 1 {
		t.Errorf("UpdateUserSSHKeys calls = %d, want 1", repo.updateSSHKeysCalls)
	}
	if repo.createSSHKeyCalls != 1 {
		t.Errorf("CreateSSHKey calls = %d, want 1", repo.createSSHKeyCalls)
	}
}

func TestEnsureSeedAdmin_NoOpWhenComplete(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("p4ss"), bcrypt.DefaultCost)
	repo := &fakeSeedAdminRepo{
		getByIdentifierResult: repository.User{ID: "u-1", Username: "admin"},
		getUserResult: repository.User{
			ID: "u-1", Username: "admin", PasswordHash: string(hash),
			EntryPassword: "x", SSHPublicKey: "p", SSHPrivateKey: "k", SSHKeyType: "ed25519",
		},
		listInboundKeys: []repository.SSHKey{
			{ID: "k-1", Purpose: "inbound", Label: "auto-generated"},
		},
	}
	if err := ensureSeedAdminWithRepo(context.Background(), newDiscardLogger(), repo, "admin", "p4ss"); err != nil {
		t.Fatalf("ensureSeedAdminWithRepo error = %v", err)
	}
	if repo.createUserCalls != 0 {
		t.Errorf("CreateUserWithRole calls = %d, want 0", repo.createUserCalls)
	}
	if repo.updateEntryCalls != 0 {
		t.Errorf("UpdateUserEntryPassword calls = %d, want 0", repo.updateEntryCalls)
	}
	if repo.updateSSHKeysCalls != 0 {
		t.Errorf("UpdateUserSSHKeys calls = %d, want 0", repo.updateSSHKeysCalls)
	}
	if repo.createSSHKeyCalls != 0 {
		t.Errorf("CreateSSHKey calls = %d, want 0", repo.createSSHKeyCalls)
	}
}

func TestEnsureSeedAdmin_FailFastOnRepoError(t *testing.T) {
	repo := &fakeSeedAdminRepo{
		getByIdentifierErr: errors.New("db connection broken"),
	}
	err := ensureSeedAdminWithRepo(context.Background(), newDiscardLogger(), repo, "admin", "p4ss")
	if err == nil {
		t.Fatal("expected error from broken repo, got nil")
	}
	if repo.createUserCalls != 0 || repo.updateEntryCalls != 0 || repo.updateSSHKeysCalls != 0 || repo.createSSHKeyCalls != 0 {
		t.Errorf("expected zero writes on fail-fast, got create=%d entry=%d ssh=%d row=%d",
			repo.createUserCalls, repo.updateEntryCalls, repo.updateSSHKeysCalls, repo.createSSHKeyCalls)
	}
}

func TestEnsureSeedAdmin_SkipsWhenCredsMissing(t *testing.T) {
	repo := &fakeSeedAdminRepo{}
	if err := ensureSeedAdminWithRepo(context.Background(), newDiscardLogger(), repo, "", ""); err != nil {
		t.Fatalf("expected nil error when admin creds unset, got %v", err)
	}
	if repo.getByIdentifierCalls != 0 {
		t.Errorf("expected zero repo calls, GetUserByLoginIdentifierForAuth calls = %d", repo.getByIdentifierCalls)
	}
	if repo.createUserCalls != 0 || repo.updateEntryCalls != 0 || repo.updateSSHKeysCalls != 0 || repo.createSSHKeyCalls != 0 {
		t.Errorf("expected zero writes when creds missing")
	}
}

func TestEnsureSeedAdmin_PreservesExistingSSHKeyWhenOnlyRowMissing(t *testing.T) {
	// 用 credgen 生成一对真实 ed25519 keys 模拟"users 表已有密钥但 ssh_keys 表缺行"的现状，
	// 这样 ComputeFingerprint 才能解析公钥（防御非空指纹守卫）。
	existingPub, existingPriv, err := credgen.GenerateSSHKeyPair("ed25519", "preexisting")
	if err != nil {
		t.Fatalf("setup: generate existing key pair: %v", err)
	}
	hashPreexisting, _ := bcrypt.GenerateFromPassword([]byte("p4ss"), bcrypt.DefaultCost)
	repo := &fakeSeedAdminRepo{
		getByIdentifierResult: repository.User{ID: "u-1", Username: "admin"},
		getUserResult: repository.User{
			ID: "u-1", Username: "admin", PasswordHash: string(hashPreexisting),
			EntryPassword: "x",
			SSHPublicKey:  existingPub,
			SSHPrivateKey: existingPriv,
			SSHKeyType:    "ed25519",
		},
		// users 表凭据已齐 → NeedSSHKeys=false；
		// 但 ssh_keys 表无 auto-generated 行 → NeedSSHKeysRow=true。
		// CreateSSHKey 必须收到 users 表里的现有 pub/priv，而非新生成的。
		listInboundKeys: nil,
	}
	if err := ensureSeedAdminWithRepo(context.Background(), newDiscardLogger(), repo, "admin", "p4ss"); err != nil {
		t.Fatalf("ensureSeedAdminWithRepo error = %v", err)
	}
	if repo.updateSSHKeysCalls != 0 {
		t.Errorf("UpdateUserSSHKeys calls = %d, want 0 (existing keys must be preserved)", repo.updateSSHKeysCalls)
	}
	if repo.createSSHKeyCalls != 1 {
		t.Fatalf("CreateSSHKey calls = %d, want 1", repo.createSSHKeyCalls)
	}
	if repo.lastCreatedSSHKeyPub != existingPub {
		t.Errorf("CreateSSHKey received pub = %q, want existing", repo.lastCreatedSSHKeyPub)
	}
	if repo.lastCreatedSSHKeyPriv != existingPriv {
		t.Errorf("CreateSSHKey received priv (truncated len=%d), want existing", len(repo.lastCreatedSSHKeyPriv))
	}
}
