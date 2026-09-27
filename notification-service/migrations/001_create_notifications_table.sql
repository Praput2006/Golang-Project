-- Postgres รันไฟล์นี้ให้อัตโนมัติตอนสร้างฐานข้อมูลครั้งแรก
-- (docker-compose mount โฟลเดอร์ migrations ไปที่ /docker-entrypoint-initdb.d)
CREATE TABLE notifications (
    id          BIGSERIAL    PRIMARY KEY,
    user_id     BIGINT       NOT NULL,                  -- Reference ID → auth_db users.id (ไม่ใช่ FK ข้าม DB)
    title       VARCHAR(100) NOT NULL,
    message     TEXT         NOT NULL,
    type        VARCHAR(30)  NOT NULL,                  -- GENERAL, BOOKING_CREATED, BOOKING_APPROVED, BOOKING_REJECTED, BOOKING_CANCELLED
    is_read     BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMP    NOT NULL DEFAULT NOW()
);

-- ผู้ใช้ดูเฉพาะของตัวเอง และกรองตามอ่านแล้ว/ยังไม่อ่านบ่อย
CREATE INDEX idx_notifications_user_read ON notifications (user_id, is_read);
