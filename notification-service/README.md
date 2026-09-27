# Notification Service

ผู้รับผิดชอบ: **นางสาววริสรา แย้มไสย** (คนที่ 4) · พอร์ต `8084` · ฐานข้อมูล `notification_db`

> อ้างอิง: เอกสาร `แบ่งงานใหม่.docx` และ `Database Design.docx` (ไม่ได้เก็บใน git ขอได้ในกลุ่มแชทของทีม)

## Endpoint (7 หมวดตาม template)

| หมวด | Endpoint | รายละเอียด |
|---|---|---|
| 1. Create | `POST /api/notifications` | Staff/Admin ส่ง Notification (ทดสอบ/ระบบเรียกใช้) |
| 2. List | `GET /api/notifications` | ดู Notification ของผู้ใช้ |
| 3. Read one | `GET /api/notifications/:id` | ดู Notification รายการเดียว (เพิ่มใหม่) |
| 4. Update | `PUT /api/notifications/:id/read` | เปลี่ยนสถานะเป็นอ่านแล้ว |
| 5. Delete | `DELETE /api/notifications/:id` | ผู้ใช้ลบ Notification ของตนเอง (เพิ่มใหม่) |
| 6. Special | (Event-driven) | รับ event จาก Booking Service (created/approved/rejected) แล้วสร้าง Notification อัตโนมัติ |
| 7. Report | `GET /api/reports/notifications/summary` | Admin ดูสถิติการแจ้งเตือน (เพิ่มใหม่) |

## ตาราง

- `notifications` — id, user_id (Reference ID ไม่ใช่ FK), title, message, type (BOOKING_CREATED / BOOKING_APPROVED / BOOKING_REJECTED / BOOKING_CANCELLED), is_read, created_at

## งานส่วนกลาง (ทุกคนทำเหมือนกัน)

- [ ] เพิ่ม route ใน `kong/kong.yml` (path `/api/notifications`, `/api/reports/notifications`)
- [ ] ต่อ Keycloak middleware (ก๊อป `auth-service/middleware.go` และดูวิธีโหลด key ด้วย library `keyfunc` ใน `auth-service/main.go`)
- [ ] เขียน Swagger/OpenAPI ของ endpoint ตัวเอง
- [ ] เขียน unit test ของ endpoint ตัวเอง
- [ ] เพิ่ม service ใน `docker-compose.example.yml` (มีโครงไว้แล้ว)

ตามเอกสารแบ่งงานใหม่ คนที่ 4 มีหน้าที่ตั้งโครงเริ่มต้นของ `kong/kong.yml` ด้วย ตอนนี้มีโครงไฟล์กับ route ของ auth-service อยู่แล้ว จะปรับหรือใช้ต่อก็ได้

## เกี่ยวข้องกับ service อื่น

- **Auth & User Service** — ผู้ใช้ดู notification ของตัวเอง ให้เรียก `GET /api/users/me` (ส่ง token ต่อไป) เพื่อหา `user_id` ส่วนการตรวจ `user_id` ตอน Staff/Admin ส่ง notification ใช้ `GET /api/users/:id`
- **Booking Service** — รับ event เมื่อสถานะการจองเปลี่ยน ⚠️ event ไม่มี token ของผู้ใช้ติดมา ต้องตกลงกันว่าจะใช้ service account ของ Keycloak หรือไม่ตรวจ user ซ้ำ
