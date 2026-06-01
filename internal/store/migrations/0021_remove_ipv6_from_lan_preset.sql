-- 0021_remove_ipv6_from_lan_preset.sql
-- IPv6 已在容器内全禁（net.ipv6.conf.all.disable_ipv6=1），
-- ULA fc00::/7 规则不生效，从局域网预设中移除。

UPDATE host_bypass_presets
SET rules = '[
  {"rule_type":"cidr","value":"10.0.0.0/8"},
  {"rule_type":"cidr","value":"172.16.0.0/12"},
  {"rule_type":"cidr","value":"192.168.0.0/16"},
  {"rule_type":"cidr","value":"100.64.0.0/10"}
]'
WHERE slug = 'lan';
