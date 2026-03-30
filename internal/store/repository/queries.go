package repository

import (
	"context"
	"encoding/json"
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
		SELECT id::text, username, status, role, COALESCE(short_id, ''), COALESCE(password_hash, ''), COALESCE(entry_password, ''), expires_at, created_at, updated_at
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
		if err := rows.Scan(&item.ID, &item.Username, &item.Status, &item.Role, &item.ShortID, &item.PasswordHash, &item.EntryPassword, &item.ExpiresAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
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
		SELECT id::text, username, status, role, COALESCE(short_id, ''), COALESCE(password_hash, ''), COALESCE(entry_password, ''), expires_at, created_at, updated_at
		FROM users WHERE id = $1
	`, userID).Scan(&item.ID, &item.Username, &item.Status, &item.Role, &item.ShortID, &item.PasswordHash, &item.EntryPassword, &item.ExpiresAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return User{}, fmt.Errorf("get user: %w", err)
	}
	return item, nil
}

func (r *Repository) CreateUser(ctx context.Context, params CreateUserParams) (User, error) {
	var item User
	if err := r.db.QueryRow(ctx, `
		INSERT INTO users (username, password_hash, status, short_id, entry_password)
		VALUES ($1, $2, 'active', $3, $4)
		RETURNING id::text, username, status, role, COALESCE(short_id, ''), COALESCE(password_hash, ''), COALESCE(entry_password, ''), expires_at, created_at, updated_at
	`, params.Username, params.PasswordHash, nullIfEmpty(params.ShortID), params.EntryPassword).Scan(
		&item.ID, &item.Username, &item.Status, &item.Role, &item.ShortID, &item.PasswordHash, &item.EntryPassword, &item.ExpiresAt, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return User{}, fmt.Errorf("create user: %w", err)
	}
	return item, nil
}

func (r *Repository) UpdateUserStatus(ctx context.Context, userID string, status string) (User, error) {
	var item User
	if err := r.db.QueryRow(ctx, `
		UPDATE users SET status = $2, updated_at = NOW() WHERE id = $1
		RETURNING id::text, username, status, role, COALESCE(short_id, ''), COALESCE(password_hash, ''), COALESCE(entry_password, ''), expires_at, created_at, updated_at
	`, userID, status).Scan(
		&item.ID, &item.Username, &item.Status, &item.Role, &item.ShortID, &item.PasswordHash, &item.EntryPassword, &item.ExpiresAt, &item.CreatedAt, &item.UpdatedAt,
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

func (r *Repository) ListHostsByUserID(ctx context.Context, userID string) ([]Host, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, user_id::text, status, COALESCE(short_id, ''), template_image_ref, home_volume_name, slot_key, timezone, hostname, memory_limit_mb, cpu_limit, disk_limit_gb, created_at, updated_at
		FROM hosts WHERE user_id = $1
		ORDER BY created_at ASC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("query hosts by user: %w", err)
	}
	defer rows.Close()

	hosts := make([]Host, 0)
	for rows.Next() {
		var item Host
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.Status, &item.ShortID, &item.TemplateImageRef,
			&item.HomeVolumeName, &item.SlotKey, &item.Timezone, &item.Hostname,
			&item.MemoryLimitMB, &item.CPULimit, &item.DiskLimitGB,
			&item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan host: %w", err)
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

func (r *Repository) ListHosts(ctx context.Context) ([]Host, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, user_id::text, status, COALESCE(short_id, ''), template_image_ref, home_volume_name, slot_key, timezone, hostname, memory_limit_mb, cpu_limit, disk_limit_gb, created_at, updated_at
		FROM hosts
		ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query hosts: %w", err)
	}
	defer rows.Close()

	hosts := make([]Host, 0)
	for rows.Next() {
		var item Host
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
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan host: %w", err)
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
	if err := r.db.QueryRow(ctx, `
		SELECT id::text, user_id::text, status, COALESCE(short_id, ''), template_image_ref, home_volume_name, slot_key, timezone, hostname, memory_limit_mb, cpu_limit, disk_limit_gb, created_at, updated_at
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
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return Host{}, fmt.Errorf("get primary host by user: %w", err)
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
		SELECT id::text, host_id::text, kind, status, requested_by, COALESCE(error_code, ''), COALESCE(error_message, ''), COALESCE(last_error_summary, ''), created_at, updated_at
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

	var item Host
	if err := r.db.QueryRow(ctx, `
		INSERT INTO hosts (user_id, status, short_id, entry_password, template_image_ref, home_volume_name, slot_key, timezone, hostname, memory_limit_mb, cpu_limit, disk_limit_gb)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
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
			updated_at = NOW()
		RETURNING id::text, user_id::text, status, COALESCE(short_id, ''), COALESCE(entry_password, ''),
		          template_image_ref, home_volume_name, slot_key, timezone, hostname,
		          memory_limit_mb, cpu_limit, disk_limit_gb, created_at, updated_at
	`,
		params.UserID,
		params.Status,
		params.ShortID,
		params.EntryPassword,
		params.TemplateImageRef,
		params.HomeVolumeName,
		params.SlotKey,
		params.Timezone,
		params.Hostname,
		memoryLimitMB,
		cpuLimit,
		diskLimitGB,
	).Scan(
		&item.ID,
		&item.UserID,
		&item.Status,
		&item.ShortID,
		&item.EntryPassword,
		&item.TemplateImageRef,
		&item.HomeVolumeName,
		&item.SlotKey,
		&item.Timezone,
		&item.Hostname,
		&item.MemoryLimitMB,
		&item.CPULimit,
		&item.DiskLimitGB,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return Host{}, fmt.Errorf("upsert host: %w", err)
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
		SELECT id::text, label, host(ip_address), provider, status, tunnel_type,
			wg_endpoint, wg_public_key, wg_preshared_key,
			COALESCE(wg_allowed_ips, '0.0.0.0/0'), wg_dns_server, wg_peer_address::text,
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
			&item.ID, &item.Label, &item.IPAddress, &item.Provider, &item.Status, &item.TunnelType,
			&item.WgEndpoint, &item.WgPublicKey, &item.WgPresharedKey,
			&item.WgAllowedIPs, &item.WgDNSServer, &item.WgPeerAddress,
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
		INSERT INTO egress_ips (label, ip_address, provider, status, wg_endpoint, wg_public_key, wg_preshared_key, wg_allowed_ips, wg_dns_server, wg_peer_address, tunnel_type, proxy_config)
		VALUES ($1, $2::inet, $3, 'available', $4, $5, $6, $7, $8, $9::cidr, $10, $11)
		RETURNING id::text, label, host(ip_address), provider, status, tunnel_type,
			wg_endpoint, wg_public_key, wg_preshared_key,
			COALESCE(wg_allowed_ips, '0.0.0.0/0'), wg_dns_server, wg_peer_address::text,
			proxy_config, created_at, updated_at
	`,
		params.Label, params.IPAddress, params.Provider,
		params.WgEndpoint, params.WgPublicKey, params.WgPresharedKey,
		params.WgAllowedIPs, params.WgDNSServer, params.WgPeerAddress,
		defaultIfEmpty(params.TunnelType, "wireguard"), params.ProxyConfig,
	).Scan(
		&item.ID, &item.Label, &item.IPAddress, &item.Provider, &item.Status, &item.TunnelType,
		&item.WgEndpoint, &item.WgPublicKey, &item.WgPresharedKey,
		&item.WgAllowedIPs, &item.WgDNSServer, &item.WgPeerAddress,
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
			wg_endpoint = $6, wg_public_key = $7, wg_preshared_key = $8,
			wg_allowed_ips = $9, wg_dns_server = $10, wg_peer_address = $11::cidr,
			tunnel_type = $12, proxy_config = $13,
			updated_at = NOW()
		WHERE id = $1
		RETURNING id::text, label, host(ip_address), provider, status, tunnel_type,
			wg_endpoint, wg_public_key, wg_preshared_key,
			COALESCE(wg_allowed_ips, '0.0.0.0/0'), wg_dns_server, wg_peer_address::text,
			proxy_config, created_at, updated_at
	`,
		egressIPID,
		params.Label, params.IPAddress, params.Provider, params.Status,
		params.WgEndpoint, params.WgPublicKey, params.WgPresharedKey,
		params.WgAllowedIPs, params.WgDNSServer, params.WgPeerAddress,
		defaultIfEmpty(params.TunnelType, "wireguard"), params.ProxyConfig,
	).Scan(
		&item.ID, &item.Label, &item.IPAddress, &item.Provider, &item.Status, &item.TunnelType,
		&item.WgEndpoint, &item.WgPublicKey, &item.WgPresharedKey,
		&item.WgAllowedIPs, &item.WgDNSServer, &item.WgPeerAddress,
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
		SELECT b.id::text, e.id::text, e.label, host(e.ip_address), e.provider, e.status, e.tunnel_type,
			e.wg_endpoint, e.wg_public_key, e.wg_preshared_key,
			COALESCE(e.wg_allowed_ips, '0.0.0.0/0'), e.wg_dns_server, e.wg_peer_address::text,
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
			&b.EgressIP.ID, &b.EgressIP.Label, &b.EgressIP.IPAddress, &b.EgressIP.Provider, &b.EgressIP.Status, &b.EgressIP.TunnelType,
			&b.EgressIP.WgEndpoint, &b.EgressIP.WgPublicKey, &b.EgressIP.WgPresharedKey,
			&b.EgressIP.WgAllowedIPs, &b.EgressIP.WgDNSServer, &b.EgressIP.WgPeerAddress,
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

func (r *Repository) ListHostsWithUsername(ctx context.Context) ([]HostWithUsername, error) {
	rows, err := r.db.Query(ctx, `
		SELECT h.id::text, h.user_id::text, h.status, COALESCE(h.short_id, ''), h.template_image_ref,
		       h.home_volume_name, h.slot_key, h.timezone, h.hostname,
		       h.memory_limit_mb, h.cpu_limit, h.disk_limit_gb,
		       h.created_at, h.updated_at, u.username,
		       e.label, host(e.ip_address)
		FROM hosts h
		JOIN users u ON u.id = h.user_id
		LEFT JOIN LATERAL (
			SELECT b.egress_ip_id FROM host_egress_bindings b
			WHERE b.host_id = h.id ORDER BY b.created_at ASC LIMIT 1
		) lb ON true
		LEFT JOIN egress_ips e ON e.id = lb.egress_ip_id
		ORDER BY h.updated_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query hosts with username: %w", err)
	}
	defer rows.Close()

	items := make([]HostWithUsername, 0)
	for rows.Next() {
		var item HostWithUsername
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.Status, &item.ShortID, &item.TemplateImageRef,
			&item.HomeVolumeName, &item.SlotKey, &item.Timezone, &item.Hostname,
			&item.MemoryLimitMB, &item.CPULimit, &item.DiskLimitGB,
			&item.CreatedAt, &item.UpdatedAt,
			&item.Username,
			&item.EgressIPLabel, &item.EgressIPAddr,
		); err != nil {
			return nil, fmt.Errorf("scan host with username: %w", err)
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
		SELECT id::text, label, host(ip_address), provider, status, tunnel_type,
			wg_endpoint, wg_public_key, wg_preshared_key,
			COALESCE(wg_allowed_ips, '0.0.0.0/0'), wg_dns_server, wg_peer_address::text,
			proxy_config, created_at, updated_at
		FROM egress_ips
		WHERE id = $1
	`, egressIPID).Scan(
		&item.ID,
		&item.Label,
		&item.IPAddress,
		&item.Provider,
		&item.Status,
		&item.TunnelType,
		&item.WgEndpoint,
		&item.WgPublicKey,
		&item.WgPresharedKey,
		&item.WgAllowedIPs,
		&item.WgDNSServer,
		&item.WgPeerAddress,
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
		SELECT e.id::text, e.label, host(e.ip_address), e.provider, e.status, e.tunnel_type,
			e.wg_endpoint, e.wg_public_key, e.wg_preshared_key,
			COALESCE(e.wg_allowed_ips, '0.0.0.0/0'), e.wg_dns_server, e.wg_peer_address::text,
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
		&item.TunnelType,
		&item.WgEndpoint,
		&item.WgPublicKey,
		&item.WgPresharedKey,
		&item.WgAllowedIPs,
		&item.WgDNSServer,
		&item.WgPeerAddress,
		&item.ProxyConfig,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return EgressIP{}, fmt.Errorf("get egress ip by host: %w", err)
	}

	return item, nil
}

func (r *Repository) GetHostWgKeys(ctx context.Context, hostID string) (wgPrivateKey, wgPublicKey string, err error) {
	var privKey, pubKey *string
	if err := r.db.QueryRow(ctx, `
		SELECT wg_private_key, wg_public_key
		FROM hosts
		WHERE id = $1
	`, hostID).Scan(&privKey, &pubKey); err != nil {
		return "", "", fmt.Errorf("get host wg keys: %w", err)
	}

	if privKey != nil {
		wgPrivateKey = *privKey
	}
	if pubKey != nil {
		wgPublicKey = *pubKey
	}
	return wgPrivateKey, wgPublicKey, nil
}

func (r *Repository) SetHostWgKeys(ctx context.Context, hostID, wgPrivateKey, wgPublicKey string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE hosts
		SET wg_private_key = $2, wg_public_key = $3, updated_at = NOW()
		WHERE id = $1
	`, hostID, wgPrivateKey, wgPublicKey)
	if err != nil {
		return fmt.Errorf("set host wg keys: %w", err)
	}

	return nil
}

func (r *Repository) GetHost(ctx context.Context, hostID string) (Host, error) {
	var item Host
	if err := r.db.QueryRow(ctx, `
		SELECT id::text, user_id::text, status, COALESCE(short_id, ''), template_image_ref, home_volume_name, slot_key, timezone, hostname, memory_limit_mb, cpu_limit, disk_limit_gb, created_at, updated_at
		FROM hosts
		WHERE id = $1
	`, hostID).Scan(
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
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return Host{}, fmt.Errorf("get host: %w", err)
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
		RETURNING id::text, host_id::text, kind, status, requested_by, COALESCE(error_code, ''), COALESCE(error_message, ''), COALESCE(last_error_summary, ''), created_at, updated_at
	`, taskID, status, nullIfEmpty(errorCode), nullIfEmpty(errorMessage), nullIfEmpty(lastErrorSummary)).Scan(
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
		return Task{}, fmt.Errorf("update task status: %w", err)
	}

	return item, nil
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
		SELECT id::text, username, status, role, COALESCE(short_id, ''), COALESCE(password_hash, ''), COALESCE(entry_password, ''), expires_at, created_at, updated_at
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
		if err := rows.Scan(&item.ID, &item.Username, &item.Status, &item.Role, &item.ShortID, &item.PasswordHash, &item.EntryPassword, &item.ExpiresAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
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
		RETURNING id::text, username, status, role, COALESCE(short_id, ''), COALESCE(password_hash, ''), COALESCE(entry_password, ''), expires_at, created_at, updated_at
	`, userID, expiresAt).Scan(
		&item.ID, &item.Username, &item.Status, &item.Role, &item.ShortID, &item.PasswordHash, &item.EntryPassword, &item.ExpiresAt, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return User{}, fmt.Errorf("update user expiry: %w", err)
	}
	return item, nil
}

func (r *Repository) ListRunningHostsByUserID(ctx context.Context, userID string) ([]Host, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, user_id::text, status, COALESCE(short_id, ''), template_image_ref, home_volume_name, slot_key, timezone, hostname, memory_limit_mb, cpu_limit, disk_limit_gb, created_at, updated_at
		FROM hosts
		WHERE user_id = $1 AND status = 'running'
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("query running hosts by user: %w", err)
	}
	defer rows.Close()

	hosts := make([]Host, 0)
	for rows.Next() {
		var item Host
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.Status, &item.ShortID, &item.TemplateImageRef,
			&item.HomeVolumeName, &item.SlotKey, &item.Timezone, &item.Hostname,
			&item.MemoryLimitMB, &item.CPULimit, &item.DiskLimitGB,
			&item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan running host: %w", err)
		}
		hosts = append(hosts, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate running hosts: %w", err)
	}
	return hosts, nil
}

func (r *Repository) ListRunningHosts(ctx context.Context) ([]Host, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, user_id::text, status, COALESCE(short_id, ''), template_image_ref, home_volume_name, slot_key, timezone, hostname, memory_limit_mb, cpu_limit, disk_limit_gb, created_at, updated_at
		FROM hosts
		WHERE status = 'running'
		ORDER BY updated_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query running hosts: %w", err)
	}
	defer rows.Close()

	hosts := make([]Host, 0)
	for rows.Next() {
		var item Host
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.Status, &item.ShortID, &item.TemplateImageRef,
			&item.HomeVolumeName, &item.SlotKey, &item.Timezone, &item.Hostname,
			&item.MemoryLimitMB, &item.CPULimit, &item.DiskLimitGB,
			&item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan running host: %w", err)
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

func (r *Repository) GetUserByShortID(ctx context.Context, shortID string) (User, error) {
	var item User
	if err := r.db.QueryRow(ctx, `
		SELECT id::text, username, status, role, COALESCE(short_id, ''), COALESCE(password_hash, ''), COALESCE(entry_password, ''), expires_at, created_at, updated_at
		FROM users WHERE short_id = $1
	`, shortID).Scan(&item.ID, &item.Username, &item.Status, &item.Role, &item.ShortID, &item.PasswordHash, &item.EntryPassword, &item.ExpiresAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return User{}, fmt.Errorf("get user by short_id: %w", err)
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

func (r *Repository) GetUserByShortIDForAuth(ctx context.Context, shortID string) (User, error) {
	var item User
	if err := r.db.QueryRow(ctx, `
		SELECT id::text, username, status, COALESCE(short_id, ''), role,
		       COALESCE(password_hash, ''), expires_at, created_at, updated_at
		FROM users WHERE short_id = $1
	`, shortID).Scan(&item.ID, &item.Username, &item.Status, &item.ShortID, &item.Role,
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

func (r *Repository) GetHostByShortID(ctx context.Context, shortID string) (HostSSHAuth, error) {
	var item HostSSHAuth
	if err := r.db.QueryRow(ctx, `
		SELECT h.id::text, h.short_id, h.entry_password, h.status, h.user_id::text, u.status, u.username
		FROM hosts h
		JOIN users u ON u.id = h.user_id
		WHERE h.short_id = $1
	`, shortID).Scan(
		&item.HostID, &item.HostShortID, &item.EntryPassword,
		&item.HostStatus, &item.UserID, &item.UserStatus, &item.Username,
	); err != nil {
		return HostSSHAuth{}, fmt.Errorf("get host by short_id: %w", err)
	}
	return item, nil
}

func (r *Repository) UpdateHostEntryPassword(ctx context.Context, hostID, password string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE hosts SET entry_password = $2, updated_at = NOW() WHERE id = $1
	`, hostID, password)
	if err != nil {
		return fmt.Errorf("update host entry password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
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
