-- Postgres รันไฟล์นี้ให้อัตโนมัติตอนสร้างฐานข้อมูลครั้งแรก
-- (docker-compose mount โฟลเดอร์ migrations ไปที่ /docker-entrypoint-initdb.d)
CREATE TABLE users (
    id            BIGSERIAL    PRIMARY KEY,
    keycloak_id   VARCHAR(64)  NOT NULL UNIQUE,
    username      VARCHAR(50)  NOT NULL UNIQUE,
    email         VARCHAR(100) NOT NULL UNIQUE,
    first_name    VARCHAR(50)  NOT NULL,
    last_name     VARCHAR(50)  NOT NULL,
    role          VARCHAR(20)  NOT NULL DEFAULT 'USER',       -- USER, STAFF, ADMIN
    status        VARCHAR(20)  NOT NULL DEFAULT 'ACTIVE',     -- ACTIVE, DEACTIVATED
    created_at    TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMP    NOT NULL DEFAULT NOW()
);

-- ผู้ดูแลระบบคนแรก ต้องตรงกับ user "uniadmin" ใน keycloak/unibook-realm.json
-- (สมัครเป็น ADMIN ผ่าน API ไม่ได้ จึงต้องมี Admin เริ่มต้นไว้เปลี่ยน role ให้คนอื่น)
INSERT INTO users (keycloak_id, username, email, first_name, last_name, role)
VALUES ('a0e1c7d2-0000-4000-8000-000000000001', 'uniadmin', 'uniadmin@unibook.local', 'System', 'Admin', 'ADMIN');
