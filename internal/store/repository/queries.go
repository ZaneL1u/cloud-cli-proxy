package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Health(ctx context.Context) error {
	return r.db.Ping(ctx)
}

func (r *Repository) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, username, status, role, COALESCE(short_id, ''), COALESCE(password_hash, ''), COALESCE(entry_password, ''), COALESCE(ssh_public_key, ''), COALESCE(ssh_private_key, ''), COALESCE(ssh_key_type, ''), expires_at, created_at, updated_at
		FROM users
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query users: %w", err)
	}
	defer rows.Close()

	users := make([]User, 0)
	for rows.Next() {
		var item User
		if err := rows.Scan(&item.ID, &item.Username, &item.Status, &item.Role, &item.ShortID, &item.PasswordHash, &item.EntryPassword, &item.SSHPublicKey, &item.SSHPrivateKey, &item.SSHKeyType, &item.ExpiresAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}

	return users, nil
}

func (r *Repository) GetUser(ctx context.Context, userID string) (User, error) {
	var item User
	if err := r.db.QueryRow(ctx, `
		SELECT id::text, username, status, role, COALESCE(short_id, ''), COALESCE(password_hash, ''), COALESCE(entry_password, ''), COALESCE(ssh_public_key, ''), COALESCE(ssh_private_key, ''), COALESCE(ssh_key_type, ''), expires_at, created_at, updated_at
		FROM users WHERE id = $1
	`, userID).Scan(&item.ID, &item.Username, &item.Status, &item.Role, &item.ShortID, &item.PasswordHash, &item.EntryPassword, &item.SSHPublicKey, &item.SSHPrivateKey, &item.SSHKeyType, &item.ExpiresAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return User{}, fmt.Errorf("get user: %w", err)
	}
	return item, nil
}

func (r *Repository) CreateUser(ctx context.Context, params CreateUserParams) (User, error) {
	var item User
	if err := r.db.QueryRow(ctx, `
		INSERT INTO users (username, password_hash, status, short_id, entry_password)
		VALUES ($1, $2, 'active', $3, $4)
		RETURNING id::text, username, status, role, COALESCE(short_id, ''), COALESCE(password_hash, ''), COALESCE(entry_password, ''), COALESCE(ssh_public_key, ''), COALESCE(ssh_private_key, ''), COALESCE(ssh_key_type, ''), expires_at, created_at, updated_at
	`, params.Username, params.PasswordHash, nullIfEmpty(params.ShortID), params.EntryPassword).Scan(
		&item.ID, &item.Username, &item.Status, &item.Role, &item.ShortID, &item.PasswordHash, &item.EntryPassword, &item.SSHPublicKey, &item.SSHPrivateKey, &item.SSHKeyType, &item.ExpiresAt, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return User{}, fmt.Errorf("create user: %w", err)
	}
	return item, nil
}

func (r *Repository) UpdateUserStatus(ctx context.Context, userID string, status string) (User, error) {
	var item User
	if err := r.db.QueryRow(ctx, `
		UPDATE users SET status = $2, updated_at = NOW() WHERE id = $1
		RETURNING id::text, username, status, role, COALESCE(short_id, ''), COALESCE(password_hash, ''), COALESCE(entry_password, ''), COALESCE(ssh_public_key, ''), COALESCE(ssh_private_key, ''), COALESCE(ssh_key_type, ''), expires_at, created_at, updated_at
	`, userID, status).Scan(
		&item.ID, &item.Username, &item.Status, &item.Role, &item.ShortID, &item.PasswordHash, &item.EntryPassword, &item.SSHPublicKey, &item.SSHPrivateKey, &item.SSHKeyType, &item.ExpiresAt, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return User{}, fmt.Errorf("update user status: %w", err)
	}
	return item, nil
}

func (r *Repository) DeleteUser(ctx context.Context, userID string) error {
	result, err := r.db.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("delete user: %w", pgx.ErrNoRows)
	}
	return nil
}

func (r *Repository) UpdateUserPassword(ctx context.Context, userID string, passwordHash string) error {
	result, err := r.db.Exec(ctx, `
		UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2
	`, passwordHash, userID)
	if err != nil {
		return fmt.Errorf("update user password: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("update user password: %w", pgx.ErrNoRows)
	}
	return nil
}

// listHostsByUserIDSQL 将 SQL 文本提升为包级常量，方便仓储层回归测试断言。
const listHostsByUserIDSQL = `
	SELECT id::text, user_id::text, status, COALESCE(short_id, ''), template_image_ref, home_volume_name, slot_key, timezone, hostname, memory_limit_mb, cpu_limit, disk_limit_gb, host_mounts, created_at, updated_at
	FROM hosts WHERE user_id = $1
	ORDER BY created_at ASC
`

func (r *Repository) ListHostsByUserID(ctx context.Context, userID string) ([]Host, error) {
	rows, err := r.db.Query(ctx, listHostsByUserIDSQL, userID)
	if err != nil {
		return nil, fmt.Errorf("query hosts by user: %w", err)
	}
	defer rows.Close()

	hosts := make([]Host, 0)
	for rows.Next() {
		var item Host
		var rawMounts json.RawMessage
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.Status, &item.ShortID, &item.TemplateImageRef,
			&item.HomeVolumeName, &item.SlotKey, &item.Timezone, &item.Hostname,
			&item.MemoryLimitMB, &item.CPULimit, &item.DiskLimitGB,
			&rawMounts,
			&item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan host: %w", err)
		}
		if len(rawMounts) > 0 {
			_ = json.Unmarshal(rawMounts, &item.HostMounts)
		}
		hosts = append(hosts, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate hosts by user: %w", err)
	}
	return hosts, nil
}

func (r *Repository) ListHostsWithEgressByUserID(ctx context.Context, userID string) ([]UserHostSummary, error) {
	rows, err := r.db.Query(ctx, `
		SELECT h.id::text, h.hostname, h.status, COALESCE(host(e.ip_address), ''), h.created_at
		FROM hosts h
		LEFT JOIN host_egress_bindings b ON b.host_id = h.id
		LEFT JOIN egress_ips e ON e.id = b.egress_ip_id
		WHERE h.user_id = $1
		ORDER BY h.created_at ASC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("query hosts with egress by user: %w", err)
	}
	defer rows.Close()

	hosts := make([]UserHostSummary, 0)
	for rows.Next() {
		var item UserHostSummary
		if err := rows.Scan(&item.ID, &item.Hostname, &item.Status, &item.EgressIP, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan user host summary: %w", err)
		}
		hosts = append(hosts, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user hosts with egress: %w", err)
	}
	return hosts, nil
}

func (r *Repository) GetDashboardStats(ctx context.Context) (DashboardStats, error) {
	var stats DashboardStats
	err := r.db.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM users WHERE status = 'active'),
			(SELECT COUNT(*) FROM hosts WHERE status = 'running'),
			(SELECT COUNT(*) FROM egress_ips WHERE status = 'available')
	`).Scan(&stats.ActiveUsers, &stats.RunningHosts, &stats.AvailableIPs)
	if err != nil {
		return DashboardStats{}, fmt.Errorf("get dashboard stats: %w", err)
	}
	return stats, nil
}

// listHostsSQL 将 SQL 文本提升为包级常量，方便仓储层回归测试断言。
const listHostsSQL = `
	SELECT id::text, user_id::text, status, COALESCE(short_id, ''), template_image_ref, home_volume_name, slot_key, timezone, hostname, memory_limit_mb, cpu_limit, disk_limit_gb, host_mounts, created_at, updated_at
	FROM hosts
	ORDER BY updated_at DESC
`

func (r *Repository) ListHosts(ctx context.Context) ([]Host, error) {
	rows, err := r.db.Query(ctx, listHostsSQL)
	if err != nil {
		return nil, fmt.Errorf("query hosts: %w", err)
	}
	defer rows.Close()

	hosts := make([]Host, 0)
	for rows.Next() {
		var item Host
		var rawMounts json.RawMessage
		if err := rows.Scan(
			&item.ID,
			&item.UserID,
			&item.Status,
			&item.ShortID,
			&item.TemplateImageRef,
			&item.HomeVolumeName,
			&item.SlotKey,
			&item.Timezone,
			&item.Hostname,
			&item.MemoryLimitMB,
			&item.CPULimit,
			&item.DiskLimitGB,
			&rawMounts,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan host: %w", err)
		}
		if len(rawMounts) > 0 {
			_ = json.Unmarshal(rawMounts, &item.HostMounts)
		}
		hosts = append(hosts, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate hosts: %w", err)
	}

	return hosts, nil
}

func (r *Repository) GetBootstrapUserByUsername(ctx context.Context, username string) (BootstrapUserAuth, error) {
	var item BootstrapUserAuth
	if err := r.db.QueryRow(ctx, `
		SELECT id::text, username, COALESCE(password_hash, ''), status, COALESCE(short_id, '')
		FROM users
		WHERE username = $1
	`, username).Scan(
		&item.UserID,
		&item.Username,
		&item.PasswordHash,
		&item.Status,
		&item.ShortID,
	); err != nil {
		return BootstrapUserAuth{}, fmt.Errorf("get bootstrap user: %w", err)
	}

	return item, nil
}

func (r *Repository) GetPrimaryHostByUserID(ctx context.Context, userID string) (Host, error) {
	var item Host
	var rawMounts json.RawMessage
	if err := r.db.QueryRow(ctx, `
		SELECT id::text, user_id::text, status, COALESCE(short_id, ''), template_image_ref, home_volume_name, slot_key, timezone, hostname, memory_limit_mb, cpu_limit, disk_limit_gb, host_mounts, created_at, updated_at
		FROM hosts
		WHERE user_id = $1 AND slot_key = 'primary'
		LIMIT 1
	`, userID).Scan(
		&item.ID,
		&item.UserID,
		&item.Status,
		&item.ShortID,
		&item.TemplateImageRef,
		&item.HomeVolumeName,
		&item.SlotKey,
		&item.Timezone,
		&item.Hostname,
		&item.MemoryLimitMB,
		&item.CPULimit,
		&item.DiskLimitGB,
		&rawMounts,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return Host{}, fmt.Errorf("get primary host by user: %w", err)
	}
	if len(rawMounts) > 0 {
		_ = json.Unmarshal(rawMounts, &item.HostMounts)
	}

	return item, nil
}

func (r *Repository) CreateTask(ctx context.Context, params CreateTaskParams) (Task, error) {
	var item Task
	if err := r.db.QueryRow(ctx, `
		INSERT INTO tasks (host_id, kind, status, requested_by, error_code, error_message, last_error_summary)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id::text, host_id::text, kind, status, requested_by, COALESCE(error_code, ''), COALESCE(error_message, ''), COALESCE(last_error_summary, ''), created_at, updated_at
	`,
		params.HostID,
		params.Kind,
		params.Status,
		params.RequestedBy,
		nullIfEmpty(params.ErrorCode),
		nullIfEmpty(params.ErrorMessage),
		nullIfEmpty(params.LastErrorSummary),
	).Scan(
		&item.ID,
		&item.HostID,
		&item.Kind,
		&item.Status,
		&item.RequestedBy,
		&item.ErrorCode,
		&item.ErrorMessage,
		&item.LastErrorSummary,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return Task{}, fmt.Errorf("create task: %w", err)
	}

	return item, nil
}

func (r *Repository) ListPendingTasks(ctx context.Context) ([]Task, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, host_id::text, kind, status, requested_by, COALESCE(error_code, ''), COALESCE(error_message, ''), COALESCE(last_error_summary, ''), created_at, updated_at
		FROM tasks
		WHERE status = 'pending'
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query pending tasks: %w", err)
	}
	defer rows.Close()

	return scanTasks(rows)
}

func (r *Repository) ListTasksWithLastErrorSummary(ctx context.Context) ([]Task, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, host_id::text, kind, status, requested_by, COALESCE(error_code, ''), COALESCE(error_message, ''), COALESCE(last_error_summary, ''), progress_percent, progress_message, created_at, updated_at
		FROM tasks
		ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query tasks: %w", err)
	}
	defer rows.Close()

	return scanTasks(rows)
}

func (r *Repository) UpsertHost(ctx context.Context, params UpsertHostParams) (Host, error) {
	// Apply defaults for zero-value resource limits
	memoryLimitMB := params.MemoryLimitMB
	if memoryLimitMB == 0 {
		memoryLimitMB = 4096
	}
	cpuLimit := params.CPULimit
	if cpuLimit == 0 {
		cpuLimit = 2.0
	}
	diskLimitGB := params.DiskLimitGB
	if diskLimitGB == 0 {
		diskLimitGB = 20
	}

	mountsJSON, err := json.Marshal(params.HostMounts)
	if err != nil {
		return Host{}, fmt.Errorf("marshal host mounts: %w", err)
	}
	portsJSON, err := json.Marshal(params.HostPorts)
	if err != nil {
		return Host{}, fmt.Errorf("marshal host ports: %w", err)
	}

	var item Host
	var rawMounts json.RawMessage
	var rawPorts json.RawMessage
	if err := r.db.QueryRow(ctx, `
		INSERT INTO hosts (user_id, status, short_id, template_image_ref, home_volume_name, slot_key, timezone, hostname, memory_limit_mb, cpu_limit, disk_limit_gb, host_mounts, host_ports)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (user_id, slot_key)
		DO UPDATE SET
			status = EXCLUDED.status,
			template_image_ref = EXCLUDED.template_image_ref,
			home_volume_name = EXCLUDED.home_volume_name,
			timezone = EXCLUDED.timezone,
			hostname = EXCLUDED.hostname,
			memory_limit_mb = EXCLUDED.memory_limit_mb,
			cpu_limit = EXCLUDED.cpu_limit,
			disk_limit_gb = EXCLUDED.disk_limit_gb,
			host_mounts = EXCLUDED.host_mounts,
			host_ports = EXCLUDED.host_ports,
			updated_at = NOW()
		RETURNING id::text, user_id::text, status, COALESCE(short_id, ''),
		          template_image_ref, home_volume_name, slot_key, timezone, hostname,
		          memory_limit_mb, cpu_limit, disk_limit_gb, host_mounts, host_ports, created_at, updated_at
	`,
		params.UserID,
		params.Status,
		params.ShortID,
		params.TemplateImageRef,
		params.HomeVolumeName,
		params.SlotKey,
		params.Timezone,
		params.Hostname,
		memoryLimitMB,
		cpuLimit,
		diskLimitGB,
		mountsJSON,
		portsJSON,
	).Scan(
		&item.ID,
		&item.UserID,
		&item.Status,
		&item.ShortID,
		&item.TemplateImageRef,
		&item.HomeVolumeName,
		&item.SlotKey,
		&item.Timezone,
		&item.Hostname,
		&item.MemoryLimitMB,
		&item.CPULimit,
		&item.DiskLimitGB,
		&rawMounts,
		&rawPorts,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return Host{}, fmt.Errorf("upsert host: %w", err)
	}
	if len(rawMounts) > 0 {
		_ = json.Unmarshal(rawMounts, &item.HostMounts)
	}
	if len(rawPorts) > 0 {
		_ = json.Unmarshal(rawPorts, &item.HostPorts)
	}

	return item, nil
}

func (r *Repository) ListHostBindings(ctx context.Context, hostID string) ([]HostBinding, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, host_id::text, egress_ip_id::text, created_at
		FROM host_egress_bindings
		WHERE host_id = $1
		ORDER BY created_at ASC
	`, hostID)
	if err != nil {
		return nil, fmt.Errorf("query host bindings: %w", err)
	}
	defer rows.Close()

	items := make([]HostBinding, 0)
	for rows.Next() {
		var item HostBinding
		if err := rows.Scan(&item.BindingID, &item.HostID, &item.EgressIPID, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan host binding: %w", err)
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate host bindings: %w", err)
	}

	return items, nil
}

func (r *Repository) ListEgressIPs(ctx context.Context) ([]EgressIP, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, label, host(ip_address), provider, status,
			proxy_config, created_at, updated_at
		FROM egress_ips
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query egress ips: %w", err)
	}
	defer rows.Close()

	items := make([]EgressIP, 0)
	for rows.Next() {
		var item EgressIP
		if err := rows.Scan(
			&item.ID, &item.Label, &item.IPAddress, &item.Provider, &item.Status,
			&item.ProxyConfig, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan egress ip: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate egress ips: %w", err)
	}
	return items, nil
}

func (r *Repository) CreateEgressIP(ctx context.Context, params CreateEgressIPParams) (EgressIP, error) {
	var item EgressIP
	if err := r.db.QueryRow(ctx, `
		INSERT INTO egress_ips (label, ip_address, provider, status, proxy_config)
		VALUES ($1, $2::inet, $3, 'available', $4)
		RETURNING id::text, label, host(ip_address), provider, status,
			proxy_config, created_at, updated_at
	`,
		params.Label, params.IPAddress, params.Provider,
		params.ProxyConfig,
	).Scan(
		&item.ID, &item.Label, &item.IPAddress, &item.Provider, &item.Status,
		&item.ProxyConfig, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return EgressIP{}, fmt.Errorf("create egress ip: %w", err)
	}
	return item, nil
}

func (r *Repository) UpdateEgressIP(ctx context.Context, egressIPID string, params UpdateEgressIPParams) (EgressIP, error) {
	var item EgressIP
	if err := r.db.QueryRow(ctx, `
		UPDATE egress_ips SET
			label = $2, ip_address = $3::inet, provider = $4, status = $5,
			proxy_config = $6,
			updated_at = NOW()
		WHERE id = $1
		RETURNING id::text, label, host(ip_address), provider, status,
			proxy_config, created_at, updated_at
	`,
		egressIPID,
		params.Label, params.IPAddress, params.Provider, params.Status,
		params.ProxyConfig,
	).Scan(
		&item.ID, &item.Label, &item.IPAddress, &item.Provider, &item.Status,
		&item.ProxyConfig, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return EgressIP{}, fmt.Errorf("update egress ip: %w", err)
	}
	return item, nil
}

func (r *Repository) DeleteEgressIP(ctx context.Context, egressIPID string) error {
	result, err := r.db.Exec(ctx, `DELETE FROM egress_ips WHERE id = $1`, egressIPID)
	if err != nil {
		return fmt.Errorf("delete egress ip: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("delete egress ip: %w", pgx.ErrNoRows)
	}
	return nil
}

func (r *Repository) BindEgressIPToHost(ctx context.Context, hostID, egressIPID string) (HostBinding, error) {
	var item HostBinding
	if err := r.db.QueryRow(ctx, `
		INSERT INTO host_egress_bindings (host_id, egress_ip_id)
		VALUES ($1, $2)
		RETURNING id::text, host_id::text, egress_ip_id::text, created_at
	`, hostID, egressIPID).Scan(
		&item.BindingID, &item.HostID, &item.EgressIPID, &item.CreatedAt,
	); err != nil {
		return HostBinding{}, fmt.Errorf("bind egress ip: %w", err)
	}
	return item, nil
}

func (r *Repository) UnbindEgressIPFromHost(ctx context.Context, bindingID string) error {
	result, err := r.db.Exec(ctx, `DELETE FROM host_egress_bindings WHERE id = $1`, bindingID)
	if err != nil {
		return fmt.Errorf("unbind egress ip: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("unbind egress ip: %w", pgx.ErrNoRows)
	}
	return nil
}

func (r *Repository) GetBindingHostID(ctx context.Context, bindingID string) (string, error) {
	var hostID string
	if err := r.db.QueryRow(ctx, `
		SELECT host_id::text FROM host_egress_bindings WHERE id = $1
	`, bindingID).Scan(&hostID); err != nil {
		return "", fmt.Errorf("get binding host id: %w", err)
	}
	return hostID, nil
}

func (r *Repository) GetHostDetail(ctx context.Context, hostID string) (HostDetail, error) {
	host, err := r.GetHost(ctx, hostID)
	if err != nil {
		return HostDetail{}, err
	}

	user, err := r.GetUser(ctx, host.UserID)
	if err != nil {
		return HostDetail{}, fmt.Errorf("get host user: %w", err)
	}

	rows, err := r.db.Query(ctx, `
		SELECT b.id::text, e.id::text, e.label, host(e.ip_address), e.provider, e.status,
			e.proxy_config, e.created_at, e.updated_at, b.created_at
		FROM host_egress_bindings b
		JOIN egress_ips e ON e.id = b.egress_ip_id
		WHERE b.host_id = $1
		ORDER BY b.created_at ASC
	`, hostID)
	if err != nil {
		return HostDetail{}, fmt.Errorf("query host bindings detail: %w", err)
	}
	defer rows.Close()

	bindings := make([]BindingWithIP, 0)
	for rows.Next() {
		var b BindingWithIP
		if err := rows.Scan(
			&b.BindingID,
			&b.EgressIP.ID, &b.EgressIP.Label, &b.EgressIP.IPAddress, &b.EgressIP.Provider, &b.EgressIP.Status,
			&b.EgressIP.ProxyConfig, &b.EgressIP.CreatedAt, &b.EgressIP.UpdatedAt, &b.CreatedAt,
		); err != nil {
			return HostDetail{}, fmt.Errorf("scan binding with ip: %w", err)
		}
		bindings = append(bindings, b)
	}
	if err := rows.Err(); err != nil {
		return HostDetail{}, fmt.Errorf("iterate host bindings detail: %w", err)
	}

	return HostDetail{Host: host, User: user, Bindings: bindings}, nil
}

// listHostsWithUsernameSQL 将 SQL 文本提升为包级常量，方便仓储层回归测试断言。
const listHostsWithUsernameSQL = `
	SELECT h.id::text, h.user_id::text, h.status, COALESCE(h.short_id, ''), h.template_image_ref,
	       h.home_volume_name, h.slot_key, h.timezone, h.hostname,
	       h.memory_limit_mb, h.cpu_limit, h.disk_limit_gb,
	       h.host_mounts, h.created_at, h.updated_at, u.username,
	       e.label, host(e.ip_address)
	FROM hosts h
	JOIN users u ON u.id = h.user_id
	LEFT JOIN LATERAL (
		SELECT b.egress_ip_id FROM host_egress_bindings b
		WHERE b.host_id = h.id ORDER BY b.created_at ASC LIMIT 1
	) lb ON true
	LEFT JOIN egress_ips e ON e.id = lb.egress_ip_id
	ORDER BY h.updated_at DESC
`

func (r *Repository) ListHostsWithUsername(ctx context.Context) ([]HostWithUsername, error) {
	rows, err := r.db.Query(ctx, listHostsWithUsernameSQL)
	if err != nil {
		return nil, fmt.Errorf("query hosts with username: %w", err)
	}
	defer rows.Close()

	items := make([]HostWithUsername, 0)
	for rows.Next() {
		var item HostWithUsername
		var rawMounts json.RawMessage
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.Status, &item.ShortID, &item.TemplateImageRef,
			&item.HomeVolumeName, &item.SlotKey, &item.Timezone, &item.Hostname,
			&item.MemoryLimitMB, &item.CPULimit, &item.DiskLimitGB,
			&rawMounts,
			&item.CreatedAt, &item.UpdatedAt,
			&item.Username,
			&item.EgressIPLabel, &item.EgressIPAddr,
		); err != nil {
			return nil, fmt.Errorf("scan host with username: %w", err)
		}
		if len(rawMounts) > 0 {
			_ = json.Unmarshal(rawMounts, &item.HostMounts)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate hosts with username: %w", err)
	}
	return items, nil
}

func (r *Repository) GetEgressIP(ctx context.Context, egressIPID string) (EgressIP, error) {
	var item EgressIP
	if err := r.db.QueryRow(ctx, `
		SELECT id::text, label, host(ip_address), provider, status,
			proxy_config, created_at, updated_at
		FROM egress_ips
		WHERE id = $1
	`, egressIPID).Scan(
		&item.ID,
		&item.Label,
		&item.IPAddress,
		&item.Provider,
		&item.Status,
		&item.ProxyConfig,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return EgressIP{}, fmt.Errorf("get egress ip: %w", err)
	}

	return item, nil
}

func (r *Repository) GetEgressIPByHost(ctx context.Context, hostID string) (EgressIP, error) {
	var item EgressIP
	if err := r.db.QueryRow(ctx, `
		SELECT e.id::text, e.label, host(e.ip_address), e.provider, e.status,
			e.proxy_config, e.created_at, e.updated_at
		FROM host_egress_bindings b
		JOIN egress_ips e ON e.id = b.egress_ip_id
		WHERE b.host_id = $1
		ORDER BY b.created_at ASC
		LIMIT 1
	`, hostID).Scan(
		&item.ID,
		&item.Label,
		&item.IPAddress,
		&item.Provider,
		&item.Status,
		&item.ProxyConfig,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return EgressIP{}, fmt.Errorf("get egress ip by host: %w", err)
	}

	return item, nil
}

// getHostSQL 将 SQL 文本提升为包级常量，方便仓储层回归测试断言。
const getHostSQL = `
	SELECT id::text, user_id::text, status, COALESCE(short_id, ''), template_image_ref, home_volume_name, slot_key, timezone, hostname, memory_limit_mb, cpu_limit, disk_limit_gb, host_mounts, host_ports, created_at, updated_at
	FROM hosts
	WHERE id = $1
`

func (r *Repository) GetHost(ctx context.Context, hostID string) (Host, error) {
	var item Host
	var rawMounts json.RawMessage
	var rawPorts json.RawMessage
	if err := r.db.QueryRow(ctx, getHostSQL, hostID).Scan(
		&item.ID,
		&item.UserID,
		&item.Status,
		&item.ShortID,
		&item.TemplateImageRef,
		&item.HomeVolumeName,
		&item.SlotKey,
		&item.Timezone,
		&item.Hostname,
		&item.MemoryLimitMB,
		&item.CPULimit,
		&item.DiskLimitGB,
		&rawMounts,
		&rawPorts,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return Host{}, fmt.Errorf("get host: %w", err)
	}
	if len(rawMounts) > 0 {
		_ = json.Unmarshal(rawMounts, &item.HostMounts)
	}
	if len(rawPorts) > 0 {
		_ = json.Unmarshal(rawPorts, &item.HostPorts)
	}

	return item, nil
}

func (r *Repository) UpdateTaskStatus(ctx context.Context, taskID, status, errorCode, errorMessage, lastErrorSummary string) (Task, error) {
	var item Task
	if err := r.db.QueryRow(ctx, `
		UPDATE tasks
		SET status = $2,
			error_code = $3,
			error_message = $4,
			last_error_summary = $5,
			updated_at = NOW()
		WHERE id = $1
		RETURNING id::text, host_id::text, kind, status, requested_by, COALESCE(error_code, ''), COALESCE(error_message, ''), COALESCE(last_error_summary, ''), progress_percent, progress_message, created_at, updated_at
	`, taskID, status, nullIfEmpty(errorCode), nullIfEmpty(errorMessage), nullIfEmpty(lastErrorSummary)).Scan(
		&item.ID,
		&item.HostID,
		&item.Kind,
		&item.Status,
		&item.RequestedBy,
		&item.ErrorCode,
		&item.ErrorMessage,
		&item.LastErrorSummary,
		&item.ProgressPercent,
		&item.ProgressMessage,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return Task{}, fmt.Errorf("update task status: %w", err)
	}

	return item, nil
}

func (r *Repository) ReportTaskProgress(ctx context.Context, taskID string, percent int, message string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE tasks
		SET progress_percent = $2,
			progress_message = $3,
			updated_at = NOW()
		WHERE id = $1
	`, taskID, percent, message)
	return err
}

func (r *Repository) RecordEvent(ctx context.Context, params RecordEventParams) (Event, error) {
	metadata := params.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}

	encoded, err := json.Marshal(metadata)
	if err != nil {
		return Event{}, fmt.Errorf("marshal event metadata: %w", err)
	}

	var item Event
	if err := r.db.QueryRow(ctx, `
		INSERT INTO events (task_id, host_id, user_id, level, type, message, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id::text, task_id::text, host_id::text, user_id::text, level, type, message, metadata, created_at
	`,
		params.TaskID,
		params.HostID,
		params.UserID,
		defaultIfEmpty(params.Level, "info"),
		params.Type,
		params.Message,
		encoded,
	).Scan(
		&item.ID,
		&item.TaskID,
		&item.HostID,
		&item.UserID,
		&item.Level,
		&item.Type,
		&item.Message,
		&encoded,
		&item.CreatedAt,
	); err != nil {
		return Event{}, fmt.Errorf("record event: %w", err)
	}

	if err := json.Unmarshal(encoded, &item.Metadata); err != nil {
		return Event{}, fmt.Errorf("decode event metadata: %w", err)
	}

	return item, nil
}

func (r *Repository) GetTaskByID(ctx context.Context, taskID string) (Task, error) {
	var item Task
	if err := r.db.QueryRow(ctx, `
		SELECT id::text, host_id::text, kind, status, requested_by,
			COALESCE(error_code, ''), COALESCE(error_message, ''),
			COALESCE(last_error_summary, ''), created_at, updated_at
		FROM tasks
		WHERE id = $1
	`, taskID).Scan(
		&item.ID,
		&item.HostID,
		&item.Kind,
		&item.Status,
		&item.RequestedBy,
		&item.ErrorCode,
		&item.ErrorMessage,
		&item.LastErrorSummary,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return Task{}, fmt.Errorf("get task by id: %w", err)
	}

	return item, nil
}

func (r *Repository) ListEventsByTaskID(ctx context.Context, taskID string, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := r.db.Query(ctx, `
		SELECT id::text, task_id::text, host_id::text, user_id::text, level, type, message, metadata, created_at
		FROM events
		WHERE task_id = $1
		ORDER BY created_at ASC
		LIMIT $2
	`, taskID, limit)
	if err != nil {
		return nil, fmt.Errorf("query events by task: %w", err)
	}
	defer rows.Close()

	items := make([]Event, 0)
	for rows.Next() {
		var item Event
		var encoded []byte
		if err := rows.Scan(
			&item.ID,
			&item.TaskID,
			&item.HostID,
			&item.UserID,
			&item.Level,
			&item.Type,
			&item.Message,
			&encoded,
			&item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		if err := json.Unmarshal(encoded, &item.Metadata); err != nil {
			return nil, fmt.Errorf("decode event metadata: %w", err)
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}

	return items, nil
}

func (r *Repository) ListExpiredActiveUsers(ctx context.Context) ([]User, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, username, status, role, COALESCE(short_id, ''), COALESCE(password_hash, ''), COALESCE(entry_password, ''), COALESCE(ssh_public_key, ''), COALESCE(ssh_private_key, ''), COALESCE(ssh_key_type, ''), expires_at, created_at, updated_at
		FROM users
		WHERE expires_at <= NOW() AND status = 'active'
		ORDER BY expires_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query expired active users: %w", err)
	}
	defer rows.Close()

	users := make([]User, 0)
	for rows.Next() {
		var item User
		if err := rows.Scan(&item.ID, &item.Username, &item.Status, &item.Role, &item.ShortID, &item.PasswordHash, &item.EntryPassword, &item.SSHPublicKey, &item.SSHPrivateKey, &item.SSHKeyType, &item.ExpiresAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan expired user: %w", err)
		}
		users = append(users, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate expired users: %w", err)
	}
	return users, nil
}

func (r *Repository) UpdateUserExpiry(ctx context.Context, userID string, expiresAt *time.Time) (User, error) {
	var item User
	if err := r.db.QueryRow(ctx, `
		UPDATE users SET expires_at = $2, updated_at = NOW() WHERE id = $1
		RETURNING id::text, username, status, role, COALESCE(short_id, ''), COALESCE(password_hash, ''), COALESCE(entry_password, ''), COALESCE(ssh_public_key, ''), COALESCE(ssh_private_key, ''), COALESCE(ssh_key_type, ''), expires_at, created_at, updated_at
	`, userID, expiresAt).Scan(
		&item.ID, &item.Username, &item.Status, &item.Role, &item.ShortID, &item.PasswordHash, &item.EntryPassword, &item.SSHPublicKey, &item.SSHPrivateKey, &item.SSHKeyType, &item.ExpiresAt, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return User{}, fmt.Errorf("update user expiry: %w", err)
	}
	return item, nil
}

// listRunningHostsByUserIDSQL 将 SQL 文本提升为包级常量，方便仓储层回归测试断言。
const listRunningHostsByUserIDSQL = `
	SELECT id::text, user_id::text, status, COALESCE(short_id, ''), template_image_ref, home_volume_name, slot_key, timezone, hostname, memory_limit_mb, cpu_limit, disk_limit_gb, host_mounts, created_at, updated_at
	FROM hosts
	WHERE user_id = $1 AND status = 'running'
`

func (r *Repository) ListRunningHostsByUserID(ctx context.Context, userID string) ([]Host, error) {
	rows, err := r.db.Query(ctx, listRunningHostsByUserIDSQL, userID)
	if err != nil {
		return nil, fmt.Errorf("query running hosts by user: %w", err)
	}
	defer rows.Close()

	hosts := make([]Host, 0)
	for rows.Next() {
		var item Host
		var rawMounts json.RawMessage
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.Status, &item.ShortID, &item.TemplateImageRef,
			&item.HomeVolumeName, &item.SlotKey, &item.Timezone, &item.Hostname,
			&item.MemoryLimitMB, &item.CPULimit, &item.DiskLimitGB,
			&rawMounts,
			&item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan running host: %w", err)
		}
		if len(rawMounts) > 0 {
			_ = json.Unmarshal(rawMounts, &item.HostMounts)
		}
		hosts = append(hosts, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate running hosts: %w", err)
	}
	return hosts, nil
}

// listRunningHostsSQL 将 SQL 文本提升为包级常量，方便仓储层回归测试断言；
// 也是 Phase 29.1 Plan 04 批量 resync 链路的事实数据源。
const listRunningHostsSQL = `
	SELECT id::text, user_id::text, status, COALESCE(short_id, ''), template_image_ref, home_volume_name, slot_key, timezone, hostname, memory_limit_mb, cpu_limit, disk_limit_gb, host_mounts, created_at, updated_at
	FROM hosts
	WHERE status = 'running'
	ORDER BY updated_at ASC
`

func (r *Repository) ListRunningHosts(ctx context.Context) ([]Host, error) {
	rows, err := r.db.Query(ctx, listRunningHostsSQL)
	if err != nil {
		return nil, fmt.Errorf("query running hosts: %w", err)
	}
	defer rows.Close()

	hosts := make([]Host, 0)
	for rows.Next() {
		var item Host
		var rawMounts json.RawMessage
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.Status, &item.ShortID, &item.TemplateImageRef,
			&item.HomeVolumeName, &item.SlotKey, &item.Timezone, &item.Hostname,
			&item.MemoryLimitMB, &item.CPULimit, &item.DiskLimitGB,
			&rawMounts,
			&item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan running host: %w", err)
		}
		if len(rawMounts) > 0 {
			_ = json.Unmarshal(rawMounts, &item.HostMounts)
		}
		hosts = append(hosts, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate running hosts: %w", err)
	}
	return hosts, nil
}

func (r *Repository) ListEvents(ctx context.Context, params ListEventsParams) (ListEventsResult, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	args := make([]any, 0)
	conditions := make([]string, 0)
	argIdx := 1

	if params.EventType != "" {
		conditions = append(conditions, fmt.Sprintf("type = $%d", argIdx))
		args = append(args, params.EventType)
		argIdx++
	}
	if params.UserID != "" {
		conditions = append(conditions, fmt.Sprintf("user_id = $%d", argIdx))
		args = append(args, params.UserID)
		argIdx++
	}
	if params.HostID != "" {
		conditions = append(conditions, fmt.Sprintf("host_id = $%d", argIdx))
		args = append(args, params.HostID)
		argIdx++
	}
	if !params.Since.IsZero() {
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", argIdx))
		args = append(args, params.Since)
		argIdx++
	}
	if !params.Until.IsZero() {
		conditions = append(conditions, fmt.Sprintf("created_at <= $%d", argIdx))
		args = append(args, params.Until)
		argIdx++
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE "
		for i, c := range conditions {
			if i > 0 {
				where += " AND "
			}
			where += c
		}
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM events %s", where)
	var total int
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return ListEventsResult{}, fmt.Errorf("count events: %w", err)
	}

	dataQuery := fmt.Sprintf(`
		SELECT id::text, task_id::text, host_id::text, user_id::text, level, type, message, metadata, created_at
		FROM events %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, where, argIdx, argIdx+1)
	args = append(args, limit, params.Offset)

	rows, err := r.db.Query(ctx, dataQuery, args...)
	if err != nil {
		return ListEventsResult{}, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	events := make([]Event, 0)
	for rows.Next() {
		var item Event
		var encoded []byte
		if err := rows.Scan(
			&item.ID, &item.TaskID, &item.HostID, &item.UserID,
			&item.Level, &item.Type, &item.Message, &encoded, &item.CreatedAt,
		); err != nil {
			return ListEventsResult{}, fmt.Errorf("scan event: %w", err)
		}
		if err := json.Unmarshal(encoded, &item.Metadata); err != nil {
			return ListEventsResult{}, fmt.Errorf("decode event metadata: %w", err)
		}
		events = append(events, item)
	}
	if err := rows.Err(); err != nil {
		return ListEventsResult{}, fmt.Errorf("iterate events: %w", err)
	}

	return ListEventsResult{Events: events, Total: total}, nil
}

func (r *Repository) GetUserByUsername(ctx context.Context, username string) (User, error) {
	var item User
	if err := r.db.QueryRow(ctx, `
		SELECT id::text, username, status, role, COALESCE(short_id, ''), COALESCE(password_hash, ''), COALESCE(entry_password, ''), COALESCE(ssh_public_key, ''), COALESCE(ssh_private_key, ''), COALESCE(ssh_key_type, ''), expires_at, created_at, updated_at
		FROM users WHERE username = $1
	`, username).Scan(&item.ID, &item.Username, &item.Status, &item.Role, &item.ShortID, &item.PasswordHash, &item.EntryPassword, &item.SSHPublicKey, &item.SSHPrivateKey, &item.SSHKeyType, &item.ExpiresAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return User{}, fmt.Errorf("get user by username: %w", err)
	}
	return item, nil
}

func (r *Repository) UpdateHostStatus(ctx context.Context, hostID string, status string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE hosts SET status = $2, updated_at = NOW() WHERE id = $1
	`, hostID, status)
	if err != nil {
		return fmt.Errorf("update host status: %w", err)
	}
	return nil
}

func (r *Repository) DeleteHost(ctx context.Context, hostID string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM host_egress_bindings WHERE host_id = $1`, hostID)
	if err != nil {
		return fmt.Errorf("delete host bindings: %w", err)
	}
	_, err = r.db.Exec(ctx, `UPDATE tasks SET host_id = NULL WHERE host_id = $1`, hostID)
	if err != nil {
		return fmt.Errorf("detach host tasks: %w", err)
	}
	_, err = r.db.Exec(ctx, `DELETE FROM hosts WHERE id = $1`, hostID)
	if err != nil {
		return fmt.Errorf("delete host: %w", err)
	}
	return nil
}

func (r *Repository) MarkStaleTasks(ctx context.Context, threshold time.Duration) ([]Task, error) {
	rows, err := r.db.Query(ctx, `
		UPDATE tasks SET status = 'failed', error_code = 'stale_timeout',
			error_message = 'task exceeded stale threshold', updated_at = NOW()
		WHERE status IN ('pending', 'running') AND updated_at < NOW() - $1::interval
		RETURNING id::text, host_id::text, kind, status, requested_by,
			COALESCE(error_code, ''), COALESCE(error_message, ''),
			COALESCE(last_error_summary, ''), created_at, updated_at
	`, threshold.String())
	if err != nil {
		return nil, fmt.Errorf("mark stale tasks: %w", err)
	}
	defer rows.Close()

	return scanTasks(rows)
}

// GetUserByLoginIdentifierForAuth 使用用户名或用户短 ID 查找账号（网页登录入口）。
// 优先匹配用户名，再匹配短 ID，兼容旧行为与 curl 入口等场景。
func (r *Repository) GetUserByLoginIdentifierForAuth(ctx context.Context, identifier string) (User, error) {
	var item User
	err := r.db.QueryRow(ctx, `
		SELECT id::text, username, status, COALESCE(short_id, ''), role,
		       COALESCE(password_hash, ''), expires_at, created_at, updated_at
		FROM users WHERE username = $1
	`, identifier).Scan(&item.ID, &item.Username, &item.Status, &item.ShortID, &item.Role,
		&item.PasswordHash, &item.ExpiresAt, &item.CreatedAt, &item.UpdatedAt)
	if err == nil {
		return item, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return User{}, fmt.Errorf("get user by username for auth: %w", err)
	}

	if err := r.db.QueryRow(ctx, `
		SELECT id::text, username, status, COALESCE(short_id, ''), role,
		       COALESCE(password_hash, ''), expires_at, created_at, updated_at
		FROM users WHERE short_id = $1
	`, identifier).Scan(&item.ID, &item.Username, &item.Status, &item.ShortID, &item.Role,
		&item.PasswordHash, &item.ExpiresAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return User{}, fmt.Errorf("get user by short_id for auth: %w", err)
	}
	return item, nil
}

func (r *Repository) CreateUserWithRole(ctx context.Context, params CreateUserWithRoleParams) (User, error) {
	var item User
	if err := r.db.QueryRow(ctx, `
		INSERT INTO users (username, password_hash, status, short_id, role)
		VALUES ($1, $2, 'active', $3, $4)
		RETURNING id::text, username, status, role, COALESCE(short_id, ''),
		          COALESCE(password_hash, ''), expires_at, created_at, updated_at
	`, params.Username, params.PasswordHash, params.ShortID, params.Role).Scan(
		&item.ID, &item.Username, &item.Status, &item.Role, &item.ShortID,
		&item.PasswordHash, &item.ExpiresAt, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return User{}, fmt.Errorf("create user with role: %w", err)
	}
	return item, nil
}

// getHostByUsernameSQL 将 SQL 文本提升为包级常量，方便数据层回归测试断言。
// 按 username 查 host，同时 SELECT ssh_private_key 供控制面私钥认证容器。
// ContainerUser 硬编码为 'workspace'，与容器镜像 entrypoint 的 CONTAINER_USER 默认值一致。
// entry_password 现归用户所有（0018 迁移后），故从 users 表读取。
const getHostByUsernameSQL = `
	SELECT h.id::text, COALESCE(u.entry_password, ''), h.status,
	       h.user_id::text, u.status, u.username,
	       'workspace',
	       COALESCE(h.template_image_ref, ''),
	       COALESCE(u.ssh_private_key, '')
	FROM hosts h
	JOIN users u ON u.id = h.user_id
	WHERE u.username = $1
`

func (r *Repository) GetHostByUsername(ctx context.Context, username string) (HostSSHAuth, error) {
	var item HostSSHAuth
	if err := r.db.QueryRow(ctx, getHostByUsernameSQL, username).Scan(
		&item.HostID, &item.EntryPassword,
		&item.HostStatus, &item.UserID, &item.UserStatus, &item.Username,
		&item.ContainerUser,
		&item.TemplateImageRef, &item.SSHPrivateKey,
	); err != nil {
		return HostSSHAuth{}, fmt.Errorf("get host by username: %w", err)
	}
	return item, nil
}

// resolveClaudeAccountByHostSQL / resolveClaudeAccountByUserFallbackSQL 实现 D-05 的确定性解析。
// 两条语句都全部使用参数化查询（T-30-01 缓解），避免 SQL 注入。
const resolveClaudeAccountByHostSQL = `
	SELECT id::text
	FROM claude_accounts
	WHERE host_id = $1
	ORDER BY created_at ASC
	LIMIT 1
`

const resolveClaudeAccountByUserFallbackSQL = `
	SELECT id::text
	FROM claude_accounts
	WHERE user_id = $1 AND host_id IS NULL
	ORDER BY created_at ASC
	LIMIT 1
`

// ResolveClaudeAccountIDForEntry 按 Phase 30 D-05 的两阶段规则返回 claude_account_id。
//
// 规则：
//  1. 优先取与当前 host 显式绑定、创建时间最早的账号；
//  2. 否则回退到当前 user 未绑定任何 host 的最早账号；
//  3. 两步都未命中时返回 (""，false，nil)，供 Wave 2 Entry API 以 omitempty 省略字段输出，
//     不视为错误（D-05 第三条）。
//
// 入参 hostID 可为空串；空串时直接跳过第一步走 fallback，用于调用方只有 user 上下文的场景。
func (r *Repository) ResolveClaudeAccountIDForEntry(ctx context.Context, userID, hostID string) (string, bool, error) {
	if userID == "" {
		return "", false, fmt.Errorf("resolve claude account: user id is required")
	}

	if hostID != "" {
		var accountID string
		err := r.db.QueryRow(ctx, resolveClaudeAccountByHostSQL, hostID).Scan(&accountID)
		switch {
		case err == nil:
			return accountID, true, nil
		case errors.Is(err, pgx.ErrNoRows):
			// fall through to user fallback
		default:
			return "", false, fmt.Errorf("resolve claude account by host: %w", err)
		}
	}

	var accountID string
	err := r.db.QueryRow(ctx, resolveClaudeAccountByUserFallbackSQL, userID).Scan(&accountID)
	switch {
	case err == nil:
		return accountID, true, nil
	case errors.Is(err, pgx.ErrNoRows):
		return "", false, nil
	default:
		return "", false, fmt.Errorf("resolve claude account by user fallback: %w", err)
	}
}

func (r *Repository) UpdateUserSSHKeys(ctx context.Context, userID, publicKey, privateKey, keyType string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE users SET ssh_public_key = $2, ssh_private_key = $3, ssh_key_type = $4, updated_at = NOW()
		WHERE id = $1
	`, userID, publicKey, privateKey, keyType)
	if err != nil {
		return fmt.Errorf("update user ssh keys: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *Repository) UpdateUserEntryPassword(ctx context.Context, userID, entryPassword string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE users SET entry_password = $2, updated_at = NOW()
		WHERE id = $1
	`, userID, entryPassword)
	if err != nil {
		return fmt.Errorf("update user entry password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *Repository) GetUserSSHKeys(ctx context.Context, userID string) (publicKey, privateKey, keyType string, err error) {
	if err = r.db.QueryRow(ctx, `
		SELECT COALESCE(ssh_public_key, ''), COALESCE(ssh_private_key, ''), COALESCE(ssh_key_type, '')
		FROM users WHERE id = $1
	`, userID).Scan(&publicKey, &privateKey, &keyType); err != nil {
		err = fmt.Errorf("get user ssh keys: %w", err)
	}
	return
}

func scanTasks(rows pgx.Rows) ([]Task, error) {
	items := make([]Task, 0)
	for rows.Next() {
		var item Task
		if err := rows.Scan(
			&item.ID,
			&item.HostID,
			&item.Kind,
			&item.Status,
			&item.RequestedBy,
			&item.ErrorCode,
			&item.ErrorMessage,
			&item.LastErrorSummary,
			&item.ProgressPercent,
			&item.ProgressMessage,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tasks: %w", err)
	}

	return items, nil
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}

	return value
}

func defaultIfEmpty(value, fallback string) string {
	if value == "" {
		return fallback
	}

	return value
}

func (r *Repository) ListSSHKeysByUser(ctx context.Context, userID string) ([]SSHKey, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, user_id::text, purpose, label, public_key, private_key, key_type, fingerprint, created_at
		FROM ssh_keys WHERE user_id = $1
		ORDER BY created_at ASC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("query ssh keys: %w", err)
	}
	defer rows.Close()

	keys := make([]SSHKey, 0)
	for rows.Next() {
		var k SSHKey
		if err := rows.Scan(&k.ID, &k.UserID, &k.Purpose, &k.Label, &k.PublicKey, &k.PrivateKey, &k.KeyType, &k.Fingerprint, &k.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan ssh key: %w", err)
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (r *Repository) ListSSHKeysByUserAndPurpose(ctx context.Context, userID, purpose string) ([]SSHKey, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, user_id::text, purpose, label, public_key, private_key, key_type, fingerprint, created_at
		FROM ssh_keys WHERE user_id = $1 AND purpose = $2
		ORDER BY created_at ASC
	`, userID, purpose)
	if err != nil {
		return nil, fmt.Errorf("query ssh keys by purpose: %w", err)
	}
	defer rows.Close()

	keys := make([]SSHKey, 0)
	for rows.Next() {
		var k SSHKey
		if err := rows.Scan(&k.ID, &k.UserID, &k.Purpose, &k.Label, &k.PublicKey, &k.PrivateKey, &k.KeyType, &k.Fingerprint, &k.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan ssh key: %w", err)
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (r *Repository) CreateSSHKey(ctx context.Context, userID, purpose, label, publicKey, privateKey, keyType, fingerprint string) (SSHKey, error) {
	var k SSHKey
	if err := r.db.QueryRow(ctx, `
		INSERT INTO ssh_keys (user_id, purpose, label, public_key, private_key, key_type, fingerprint)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id::text, user_id::text, purpose, label, public_key, private_key, key_type, fingerprint, created_at
	`, userID, purpose, label, publicKey, privateKey, keyType, fingerprint).Scan(
		&k.ID, &k.UserID, &k.Purpose, &k.Label, &k.PublicKey, &k.PrivateKey, &k.KeyType, &k.Fingerprint, &k.CreatedAt,
	); err != nil {
		return SSHKey{}, fmt.Errorf("create ssh key: %w", err)
	}
	return k, nil
}

func (r *Repository) DeleteSSHKey(ctx context.Context, keyID, userID string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM ssh_keys WHERE id = $1 AND user_id = $2`, keyID, userID)
	if err != nil {
		return fmt.Errorf("delete ssh key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// DeleteAutoGeneratedInboundSSHKeys 删除 user 的自动生成入站密钥（label='auto-generated'）。
// 用于凭据重生路径：覆盖旧 auto-generated 行，但保留用户手动添加的入站密钥。
func (r *Repository) DeleteAutoGeneratedInboundSSHKeys(ctx context.Context, userID string) error {
	_, err := r.db.Exec(ctx, `
		DELETE FROM ssh_keys
		WHERE user_id = $1 AND purpose = 'inbound' AND label = 'auto-generated'
	`, userID)
	if err != nil {
		return fmt.Errorf("delete auto-generated inbound ssh keys: %w", err)
	}
	return nil
}

func (r *Repository) UpdateHostMounts(ctx context.Context, hostID string, mounts HostMounts) error {
	data, err := json.Marshal(mounts)
	if err != nil {
		return fmt.Errorf("marshal host mounts: %w", err)
	}
	_, err = r.db.Exec(ctx, `UPDATE hosts SET host_mounts = $1, updated_at = NOW() WHERE id = $2`, data, hostID)
	return err
}

func (r *Repository) UpdateHostPorts(ctx context.Context, hostID string, ports HostPorts) error {
	data, err := json.Marshal(ports)
	if err != nil {
		return fmt.Errorf("marshal host ports: %w", err)
	}
	_, err = r.db.Exec(ctx, `UPDATE hosts SET host_ports = $1, updated_at = NOW() WHERE id = $2`, data, hostID)
	return err
}

func (r *Repository) GetSSHKey(ctx context.Context, keyID string) (SSHKey, error) {
	var k SSHKey
	if err := r.db.QueryRow(ctx, `
		SELECT id::text, user_id::text, purpose, label, public_key, private_key, key_type, fingerprint, created_at
		FROM ssh_keys WHERE id = $1
	`, keyID).Scan(&k.ID, &k.UserID, &k.Purpose, &k.Label, &k.PublicKey, &k.PrivateKey, &k.KeyType, &k.Fingerprint, &k.CreatedAt); err != nil {
		return SSHKey{}, fmt.Errorf("get ssh key: %w", err)
	}
	return k, nil
}

const upsertClaudeAccountPersistentVolumeNameSQL = `
	UPDATE claude_accounts
	SET persistent_volume_name = $2, updated_at = NOW()
	WHERE id = $1 AND persistent_volume_name IS NULL
`

const checkClaudeAccountPersistentVolumeNameSQL = `
	SELECT COALESCE(persistent_volume_name, '')
	FROM claude_accounts
	WHERE id = $1
`

// UpsertClaudeAccountPersistentVolumeName 实现 Phase 33 D-06 三态语义：
//   - persistent_volume_name IS NULL → 写入 volumeName（一次往返）
//   - 已是相同 volumeName → 跳过返回 nil
//   - 已是其他值（冲突） → 返回错误（调用方写 audit）
//
// 不允许从已分配回写 NULL（与 Phase 30 D-02 三态消除一致）。
func (r *Repository) UpsertClaudeAccountPersistentVolumeName(ctx context.Context, accountID, volumeName string) error {
	if accountID == "" || volumeName == "" {
		return fmt.Errorf("upsert claude_account persistent_volume_name: empty arg")
	}
	tag, err := r.db.Exec(ctx, upsertClaudeAccountPersistentVolumeNameSQL, accountID, volumeName)
	if err != nil {
		return fmt.Errorf("update persistent_volume_name: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	var current string
	if err := r.db.QueryRow(ctx, checkClaudeAccountPersistentVolumeNameSQL, accountID).Scan(&current); err != nil {
		return fmt.Errorf("verify persistent_volume_name: %w", err)
	}
	if current == volumeName {
		return nil
	}
	return fmt.Errorf("persistent_volume_name conflict: current=%q want=%q", current, volumeName)
}

const getHostWithClaudeAccountSQL = `
	SELECT
		h.id::text, h.user_id::text, h.status, COALESCE(h.short_id, ''),
		h.template_image_ref, h.home_volume_name,
		h.slot_key, h.timezone, h.hostname, h.memory_limit_mb, h.cpu_limit,
		h.disk_limit_gb, h.host_mounts, h.created_at, h.updated_at,
		COALESCE(ca.persistent_volume_name, '')
	FROM hosts h
	LEFT JOIN claude_accounts ca ON ca.host_id = h.id
	WHERE h.id = $1
	ORDER BY ca.created_at ASC
	LIMIT 1
`

// GetHostWithClaudeAccount D-23：单次 LEFT JOIN 返回 host + 可能 NULL 的 persistent_volume_name。
// 与 GetHost / ListHostsWithUsername 等 6 个既有 SELECT 解耦，不修改 Phase 29.1 已锁定的查询。
func (r *Repository) GetHostWithClaudeAccount(ctx context.Context, hostID string) (HostWithClaudeAccount, error) {
	var item HostWithClaudeAccount
	var rawMounts json.RawMessage
	if err := r.db.QueryRow(ctx, getHostWithClaudeAccountSQL, hostID).Scan(
		&item.ID, &item.UserID, &item.Status, &item.ShortID,
		&item.TemplateImageRef, &item.HomeVolumeName,
		&item.SlotKey, &item.Timezone, &item.Hostname,
		&item.MemoryLimitMB, &item.CPULimit, &item.DiskLimitGB,
		&rawMounts,
		&item.CreatedAt, &item.UpdatedAt,
		&item.PersistentVolumeName,
	); err != nil {
		return HostWithClaudeAccount{}, fmt.Errorf("get host with claude_account: %w", err)
	}
	if len(rawMounts) > 0 {
		_ = json.Unmarshal(rawMounts, &item.HostMounts)
	}
	return item, nil
}

// BeginTx 暴露 pgx 事务给 admin handler（D-18），避免把 *pgxpool.Pool 泄漏到 control plane。
// 与 internal/store/migrator/migrator.go:46 唯一既有 r.db.Begin 调用点对齐。
func (r *Repository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.db.Begin(ctx)
}

const lockClaudeAccountForDeleteSQL = `
	SELECT id::text, COALESCE(persistent_volume_name, '')
	FROM claude_accounts
	WHERE id = $1
	FOR UPDATE
`

const deleteClaudeAccountSQL = `DELETE FROM claude_accounts WHERE id = $1`

// LockClaudeAccountForDelete D-18 强一致路径第 2 步：BEGIN 后行锁 + 读 volume 名。
// 包级函数（非 method）以便 handler 显式持有 tx ref。
func LockClaudeAccountForDelete(ctx context.Context, tx pgx.Tx, id string) (accountID, volumeName string, err error) {
	err = tx.QueryRow(ctx, lockClaudeAccountForDeleteSQL, id).Scan(&accountID, &volumeName)
	return
}

// DeleteClaudeAccountTx 在事务内删除 claude_account 行；RowsAffected==0 返回 pgx.ErrNoRows。
func DeleteClaudeAccountTx(ctx context.Context, tx pgx.Tx, id string) error {
	tag, err := tx.Exec(ctx, deleteClaudeAccountSQL, id)
	if err != nil {
		return fmt.Errorf("delete claude_account: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
