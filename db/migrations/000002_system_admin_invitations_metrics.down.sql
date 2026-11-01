DROP FUNCTION IF EXISTS ru_lower(text);
DROP TABLE IF EXISTS system_metric_samples;
DROP TABLE IF EXISTS service_heartbeats;
DROP TABLE IF EXISTS invitations;
DELETE FROM user_roles WHERE role_id IN (SELECT id FROM roles WHERE code = 'system_admin');
DELETE FROM roles WHERE code = 'system_admin';
ALTER TABLE roles DROP CONSTRAINT roles_code_check;
ALTER TABLE roles ADD CONSTRAINT roles_code_check CHECK (code IN
    ('administrator','teacher','class_teacher','student','parent','principal'));
ALTER TABLE users DROP COLUMN IF EXISTS last_login_at;
