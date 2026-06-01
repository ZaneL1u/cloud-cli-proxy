-- 新增 detected_ip_address 列，存储探针或运行时验证检测到的真实出口 IP。
-- ip_address 保留为用户配置的原始值（可能是占位符 0.0.0.0），
-- detected_ip_address 为实际检测值，NULL 表示尚未检测。
ALTER TABLE egress_ips ADD COLUMN detected_ip_address TEXT;
