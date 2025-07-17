CREATE DATABASE IF NOT EXISTS rinha_db;

USE rinha_db;

CREATE TABLE IF NOT EXISTS payments (
    id INT AUTO_INCREMENT PRIMARY KEY,
    correlation_id VARCHAR(255) NOT NULL,
    amount DECIMAL(10,2) NOT NULL,
    is_default_processor BOOLEAN NOT NULL,
    requested_at DATETIME NOT NULL
);
CREATE INDEX idx_correlation_id ON payments (correlation_id);
CREATE INDEX idx_requested_at ON payments (requested_at);
CREATE INDEX idx_processor_requested ON payments (is_default_processor, requested_at);
