-- MySQL 8.4, schema version 1. Create the database before running this script.
-- DDL is deliberately separate from policy transactions (MySQL DDL commits).
CREATE TABLE IF NOT EXISTS speedle_store_meta (
  id TINYINT NOT NULL PRIMARY KEY,
  schema_version INT NOT NULL,
  revision BIGINT NOT NULL DEFAULT 0,
  CHECK (id = 1), CHECK (revision >= 0)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS speedle_services (
  name VARCHAR(255) NOT NULL PRIMARY KEY,
  service_type VARCHAR(255) NOT NULL,
  metadata JSON NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS speedle_policies (
  service_name VARCHAR(255) NOT NULL,
  id VARCHAR(255) NOT NULL,
  name LONGTEXT NOT NULL,
  effect VARCHAR(16) NOT NULL,
  permissions JSON NOT NULL,
  principals JSON NOT NULL,
  condition_expr LONGTEXT NOT NULL,
  metadata JSON NOT NULL,
  PRIMARY KEY (service_name, id),
  FOREIGN KEY (service_name) REFERENCES speedle_services(name) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS speedle_role_policies (
  service_name VARCHAR(255) NOT NULL,
  id VARCHAR(255) NOT NULL,
  name LONGTEXT NOT NULL,
  effect VARCHAR(16) NOT NULL,
  roles JSON NOT NULL,
  principals JSON NOT NULL,
  resources JSON NOT NULL,
  resource_expressions JSON NOT NULL,
  condition_expr LONGTEXT NOT NULL,
  metadata JSON NOT NULL,
  PRIMARY KEY (service_name, id),
  FOREIGN KEY (service_name) REFERENCES speedle_services(name) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

CREATE TABLE IF NOT EXISTS speedle_functions (
  name VARCHAR(255) NOT NULL PRIMARY KEY,
  description LONGTEXT NOT NULL,
  func_url LONGTEXT NOT NULL,
  local_func_url LONGTEXT NOT NULL,
  ca LONGTEXT NOT NULL,
  result_cachable BOOLEAN NOT NULL,
  result_ttl BIGINT NOT NULL,
  metadata JSON NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin;

INSERT IGNORE INTO speedle_store_meta (id, schema_version, revision) VALUES (1, 1, 0);
