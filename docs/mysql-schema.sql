CREATE TABLE users (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  username VARCHAR(64) NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  display_name VARCHAR(128) NOT NULL,
  role VARCHAR(32) NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at DATETIME(3) NULL,
  updated_at DATETIME(3) NULL,
  UNIQUE KEY idx_users_username (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE user_sessions (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  token_hash VARCHAR(128) NOT NULL,
  expires_at DATETIME(3) NOT NULL,
  revoked_at DATETIME(3) NULL,
  created_at DATETIME(3) NULL,
  updated_at DATETIME(3) NULL,
  UNIQUE KEY idx_user_sessions_token_hash (token_hash),
  KEY idx_user_sessions_user_id (user_id),
  KEY idx_user_sessions_expires_at (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE billing_accounts (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  account_id VARCHAR(64) NOT NULL,
  name VARCHAR(128) NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  current_period_id BIGINT UNSIGNED NULL,
  created_at DATETIME(3) NULL,
  updated_at DATETIME(3) NULL,
  UNIQUE KEY idx_billing_accounts_account_id (account_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE billing_api_keys (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  account_id BIGINT UNSIGNED NOT NULL,
  `key` VARCHAR(255) NOT NULL,
  name VARCHAR(128) NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at DATETIME(3) NULL,
  updated_at DATETIME(3) NULL,
  UNIQUE KEY idx_billing_api_keys_key (`key`),
  KEY idx_billing_api_keys_account_id (account_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE account_quota_periods (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  account_id BIGINT UNSIGNED NOT NULL,
  quota_micro_credits BIGINT NOT NULL,
  used_micro_credits BIGINT NOT NULL DEFAULT 0,
  reserved_micro_credits BIGINT NOT NULL DEFAULT 0,
  period_start_at DATETIME(3) NOT NULL,
  period_end_at DATETIME(3) NOT NULL,
  status VARCHAR(32) NOT NULL,
  created_at DATETIME(3) NULL,
  updated_at DATETIME(3) NULL,
  KEY idx_account_quota_periods_account_id (account_id),
  KEY idx_account_quota_periods_period_start_at (period_start_at),
  KEY idx_account_quota_periods_period_end_at (period_end_at),
  KEY idx_account_quota_periods_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE account_recharges (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  account_id BIGINT UNSIGNED NOT NULL,
  mode VARCHAR(64) NOT NULL,
  quota_micro_credits BIGINT NOT NULL,
  period_days BIGINT NOT NULL,
  created_at DATETIME(3) NULL,
  KEY idx_account_recharges_account_id (account_id),
  KEY idx_account_recharges_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE billing_reservations (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  reservation_id VARCHAR(64) NOT NULL,
  request_id VARCHAR(128) NOT NULL,
  trace_id VARCHAR(128) NULL,
  account_id BIGINT UNSIGNED NOT NULL,
  period_id BIGINT UNSIGNED NOT NULL,
  endpoint VARCHAR(255) NOT NULL,
  model VARCHAR(128) NULL,
  settlement_policy VARCHAR(64) NOT NULL,
  policy_version VARCHAR(64) NOT NULL,
  price_snapshot TEXT NULL,
  estimated_micro_credits BIGINT NOT NULL,
  reserved_micro_credits BIGINT NOT NULL,
  final_micro_credits BIGINT NOT NULL DEFAULT 0,
  status VARCHAR(32) NOT NULL,
  release_reason VARCHAR(128) NULL,
  expires_at DATETIME(3) NOT NULL,
  created_at DATETIME(3) NULL,
  updated_at DATETIME(3) NULL,
  UNIQUE KEY idx_billing_reservations_reservation_id (reservation_id),
  KEY idx_billing_reservations_request_id (request_id),
  KEY idx_billing_reservations_account_id (account_id),
  KEY idx_billing_reservations_period_id (period_id),
  KEY idx_billing_reservations_status (status),
  KEY idx_billing_reservations_expires_at (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE billing_ledger_records (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  account_id BIGINT UNSIGNED NOT NULL,
  period_id BIGINT UNSIGNED NOT NULL,
  reservation_id VARCHAR(64) NULL,
  entry_type VARCHAR(32) NOT NULL,
  delta_reserved_micro BIGINT NOT NULL DEFAULT 0,
  delta_used_micro BIGINT NOT NULL DEFAULT 0,
  balance_reserved_micro BIGINT NOT NULL DEFAULT 0,
  balance_used_micro BIGINT NOT NULL DEFAULT 0,
  request_id VARCHAR(128) NULL,
  trace_id VARCHAR(128) NULL,
  created_at DATETIME(3) NULL,
  KEY idx_billing_ledger_records_account_id (account_id),
  KEY idx_billing_ledger_records_period_id (period_id),
  KEY idx_billing_ledger_records_reservation_id (reservation_id),
  KEY idx_billing_ledger_records_request_id (request_id),
  KEY idx_billing_ledger_records_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE upstream_pools (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  pool_id VARCHAR(64) NOT NULL,
  name VARCHAR(128) NOT NULL,
  source_type VARCHAR(32) NOT NULL,
  base_url VARCHAR(512) NULL,
  rust_grpc_addr VARCHAR(255) NULL,
  monthly_quota_micro_credits BIGINT NOT NULL,
  oversell_percent DOUBLE NOT NULL DEFAULT 0,
  exhaust_threshold DOUBLE NOT NULL DEFAULT 0.98,
  active_cycle_id BIGINT UNSIGNED NULL,
  disabled_by_admin BOOLEAN NOT NULL DEFAULT FALSE,
  frozen_by_error BOOLEAN NOT NULL DEFAULT FALSE,
  cooldown_until DATETIME(3) NULL,
  last_error_code VARCHAR(128) NULL,
  last_error_at DATETIME(3) NULL,
  created_at DATETIME(3) NULL,
  updated_at DATETIME(3) NULL,
  UNIQUE KEY idx_upstream_pools_pool_id (pool_id),
  KEY idx_upstream_pools_source_type (source_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE upstream_api_accounts (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  pool_id BIGINT UNSIGNED NOT NULL,
  account_ref VARCHAR(128) NOT NULL,
  api_key VARCHAR(1024) NULL,
  monthly_quota_micro_credits BIGINT NOT NULL,
  used_micro_credits BIGINT NOT NULL DEFAULT 0,
  reserved_micro_credits BIGINT NOT NULL DEFAULT 0,
  priority BIGINT NOT NULL DEFAULT 100,
  disabled_by_admin BOOLEAN NOT NULL DEFAULT FALSE,
  frozen_by_error BOOLEAN NOT NULL DEFAULT FALSE,
  cooldown_until DATETIME(3) NULL,
  last_error_code VARCHAR(128) NULL,
  last_error_at DATETIME(3) NULL,
  created_at DATETIME(3) NULL,
  updated_at DATETIME(3) NULL,
  KEY idx_upstream_api_accounts_pool_id (pool_id),
  KEY idx_upstream_api_accounts_priority (priority)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE pool_quota_cycles (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  pool_id BIGINT UNSIGNED NOT NULL,
  quota_micro_credits BIGINT NOT NULL,
  used_micro_credits BIGINT NOT NULL DEFAULT 0,
  reserved_micro_credits BIGINT NOT NULL DEFAULT 0,
  cycle_start_at DATETIME(3) NOT NULL,
  cycle_end_at DATETIME(3) NOT NULL,
  status VARCHAR(32) NOT NULL,
  reconcile_state VARCHAR(32) NULL,
  created_at DATETIME(3) NULL,
  updated_at DATETIME(3) NULL,
  KEY idx_pool_quota_cycles_pool_id (pool_id),
  KEY idx_pool_quota_cycles_cycle_start_at (cycle_start_at),
  KEY idx_pool_quota_cycles_cycle_end_at (cycle_end_at),
  KEY idx_pool_quota_cycles_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE account_pool_assignments (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  customer_account_id BIGINT UNSIGNED NOT NULL,
  pool_id BIGINT UNSIGNED NOT NULL,
  upstream_account_id BIGINT UNSIGNED NOT NULL,
  sold_capacity_micro_credits BIGINT NOT NULL DEFAULT 0,
  active BOOLEAN NOT NULL DEFAULT TRUE,
  reason VARCHAR(128) NULL,
  created_at DATETIME(3) NULL,
  updated_at DATETIME(3) NULL,
  KEY idx_account_pool_assignments_customer_account_id (customer_account_id),
  KEY idx_account_pool_assignments_pool_id (pool_id),
  KEY idx_account_pool_assignments_upstream_account_id (upstream_account_id),
  KEY idx_account_pool_assignments_active (active)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE upstream_capacity_reservations (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  reservation_id VARCHAR(64) NOT NULL,
  request_id VARCHAR(128) NOT NULL,
  trace_id VARCHAR(128) NULL,
  customer_account_id BIGINT UNSIGNED NOT NULL,
  pool_id BIGINT UNSIGNED NOT NULL,
  pool_cycle_id BIGINT UNSIGNED NOT NULL,
  upstream_account_id BIGINT UNSIGNED NOT NULL,
  estimated_micro_credits BIGINT NOT NULL,
  reserved_micro_credits BIGINT NOT NULL,
  final_micro_credits BIGINT NOT NULL DEFAULT 0,
  status VARCHAR(32) NOT NULL,
  release_reason VARCHAR(128) NULL,
  expires_at DATETIME(3) NOT NULL,
  created_at DATETIME(3) NULL,
  updated_at DATETIME(3) NULL,
  UNIQUE KEY idx_upstream_capacity_reservations_reservation_id (reservation_id),
  KEY idx_upstream_capacity_reservations_request_id (request_id),
  KEY idx_upstream_capacity_reservations_customer_account_id (customer_account_id),
  KEY idx_upstream_capacity_reservations_pool_id (pool_id),
  KEY idx_upstream_capacity_reservations_pool_cycle_id (pool_cycle_id),
  KEY idx_upstream_capacity_reservations_upstream_account_id (upstream_account_id),
  KEY idx_upstream_capacity_reservations_status (status),
  KEY idx_upstream_capacity_reservations_expires_at (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE upstream_resource_owners (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  resource_type VARCHAR(64) NOT NULL,
  resource_id VARCHAR(255) NOT NULL,
  customer_account_id BIGINT UNSIGNED NOT NULL,
  pool_id BIGINT UNSIGNED NOT NULL,
  upstream_account_id BIGINT UNSIGNED NOT NULL,
  source_request_id VARCHAR(128) NULL,
  created_at DATETIME(3) NULL,
  updated_at DATETIME(3) NULL,
  UNIQUE KEY idx_resource_owner (resource_type, resource_id),
  KEY idx_upstream_resource_owners_customer_account_id (customer_account_id),
  KEY idx_upstream_resource_owners_pool_id (pool_id),
  KEY idx_upstream_resource_owners_upstream_account_id (upstream_account_id),
  KEY idx_upstream_resource_owners_source_request_id (source_request_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE quota_reconcile_runs (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  pool_id BIGINT UNSIGNED NOT NULL,
  upstream_account_id BIGINT UNSIGNED NULL,
  observed_used_micro_credits BIGINT NOT NULL DEFAULT 0,
  observed_cost_micro_credits BIGINT NOT NULL DEFAULT 0,
  provider_state VARCHAR(64) NULL,
  confidence DOUBLE NULL,
  observed_at DATETIME(3) NULL,
  raw_error TEXT NULL,
  created_at DATETIME(3) NULL,
  KEY idx_quota_reconcile_runs_pool_id (pool_id),
  KEY idx_quota_reconcile_runs_upstream_account_id (upstream_account_id),
  KEY idx_quota_reconcile_runs_observed_at (observed_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

