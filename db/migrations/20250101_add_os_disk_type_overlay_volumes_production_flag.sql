-- Add OS and disk_type to projects
ALTER TABLE projects
    ADD COLUMN os VARCHAR(255) DEFAULT 'cos-125-19216-104-74' AFTER disk_size_gb,
    ADD COLUMN disk_type VARCHAR(255) DEFAULT 'hyperdisk-balanced' AFTER os;

-- Add OS, overlay_volumes, and is_production to sites
ALTER TABLE sites
    ADD COLUMN overlay_volumes JSON DEFAULT ('[]') AFTER rollout_cmd,
    ADD COLUMN os VARCHAR(255) DEFAULT 'cos-125-19216-104-74' AFTER overlay_volumes,
    ADD COLUMN is_production BOOLEAN DEFAULT FALSE AFTER os;
