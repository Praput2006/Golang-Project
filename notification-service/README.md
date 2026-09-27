# Notification Service

ผู้รับผิดชอบ: **นางสาววริสรา แย้มไสย** (คนที่ 4) · พอร์ต `8084` · ฐานข้อมูล `notification_db`

> อ้างอิง: เอกสาร `แบ่งงานใหม่.docx` และ `Database Design.docx` (ไม่ได้เก็บใน git ขอได้ในกลุ่มแชทของทีม)

## Endpoint (7 หมวดตาม template)

| หมวด | Endpoint | สิทธิ์ | รายละเอียด |
|---|---|---|---|
| 1. Create | `POST /api/notifications` | STAFF, ADMIN | Staff/Admin ส่ง Notification (ตรวจว่ามี `user_id` จริงผ่าน `GET /api/users/:id`) |
| 2. List | `GET /api/notifications?is_read=&type=&page=&limit=` | login แล้ว | ดู Notification ของตัวเอง |
| 3. Read one | `GET /api/notifications/:id` | เจ้าของ | ดู Notification รายการเดียว |
| 4. Update | `PUT /api/notifications/:id/read` | เจ้าของ | เปลี่ยนสถานะเป็นอ่านแล้ว |
| 5. Delete | `DELETE /api/notifications/:id` | เจ้าของ | ลบ Notification ของตนเอง |
| 6. Special | `POST /internal/events` | `X-Internal-Key` | รับ event จาก Booking Service (created/approved/rejected) แล้วสร้าง Notification อัตโนมัติ — ดู [EVENTS.md](EVENTS.md) |
| 7. Report | `GET /api/reports/notifications/summary` | ADMIN | สถิติการแจ้งเตือน (total, read, unread, read_rate, by_type) |

ทุก role (รวม ADMIN) ดู/อ่าน/ลบได้เฉพาะ notification ของตัวเอง

เอกสาร Swagger: http://localhost:8084/swagger

## ตาราง

- `notifications` — id, user_id (Reference ID → `users.id` ใน auth_db ไม่ใช่ FK), title, message, type (GENERAL / BOOKING_CREATED / BOOKING_APPROVED / BOOKING_REJECTED / BOOKING_CANCELLED), is_read, created_at
- สร้างจาก `migrations/001_create_notifications_table.sql` (Postgres รันให้เองครั้งแรกที่สร้างฐานข้อมูล)
- เพิ่ม type `GENERAL` จาก Database Design สำหรับข้อความที่ Staff/Admin ส่งเอง

## งานส่วนกลาง (ทุกคนทำเหมือนกัน)

- [x] เพิ่ม route ใน `kong/kong.yml` (path `/api/notifications`, `/api/reports/notifications` — ไม่เปิด `/internal`)
- [x] ต่อ Keycloak middleware (ก๊อป `auth-service/middleware.go` + โหลด key ด้วย `keyfunc`)
- [x] เขียน Swagger/OpenAPI ของ endpoint ตัวเอง (`docs/openapi.yaml`)
- [x] เขียน unit test ของ endpoint ตัวเอง (`handlers_test.go`, `middleware_test.go`)
- [x] เพิ่ม service ใน `docker-compose.example.yml`

## วิธีรัน

**รันใน Docker ด้วย compose กลาง** (ที่ root ของ repo — ตอนนี้ facility/booking ยังไม่มี Dockerfile จึงเลือกรันเฉพาะ service ที่พร้อม)

```bash
docker compose -f docker-compose.example.yml -p unibook up -d --build auth-db keycloak auth-service notification-db notification-service
docker compose -f docker-compose.example.yml -p unibook up -d --no-deps kong
```

**รันตอนเขียนโค้ด** (Keycloak + auth-service ใน Docker ส่วนตัว Go รันในเครื่อง)

```bash
cd auth-service && docker compose up -d                 # keycloak :8080, auth-service :8081
docker run -d --name notif-pg-test -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=notification_db -p 5434:5432 postgres:16
docker exec -i notif-pg-test psql -U postgres -d notification_db < notification-service/migrations/001_create_notifications_table.sql
cd notification-service && cp .env.example .env && go run .
```

ห้ามเปิด `auth-service/docker-compose.yml` พร้อมกับ compose กลาง (ใช้พอร์ต 8080/8081 ชนกัน)

## ทดสอบ

```bash
go test ./...
```

unit test รันได้ทันทีโดยไม่ต้องเปิด Docker — ใช้ SQLite ในหน่วยความจำแทน PostgreSQL, จำลอง auth-service ด้วย `httptest` และเซ็น token ด้วย RSA key ปลอมแบบเดียวกับ auth-service

ทดสอบ API จริงด้วย VS Code extension **REST Client** ที่ไฟล์ `requests.http` (login ผ่าน Keycloak ให้เอง)

## Environment

| ตัวแปร | ตัวอย่าง | ใช้ทำอะไร |
|---|---|---|
| `DATABASE_URL` | `postgres://postgres:notificationpass@notification-db:5432/notification_db` | ฐานข้อมูล |
| `KEYCLOAK_URL`, `KEYCLOAK_REALM` | `http://keycloak:8080`, `unibook` | โหลด public key ตรวจ token |
| `KEYCLOAK_ISSUER` | `http://localhost:8080/realms/unibook` | issuer ใน token (ผู้ใช้ login ผ่าน localhost) |
| `AUTH_SERVICE_URL` | `http://auth-service:8081` | หา `users.id` ของคนที่ login / ตรวจ `user_id` |
| `INTERNAL_API_KEY` | `dev-internal-key` | ใช้ร่วมกับ booking-service สำหรับ `/internal/events` |
| `PORT` | `8084` (ค่าเริ่มต้น) | |

## เกี่ยวข้องกับ service อื่น

- **Auth & User Service** — token มีแค่ `keycloak_id` จึงเรียก `GET /api/users/me` (ส่ง token ต่อไป) เพื่อหา `user_id` ส่วนการตรวจ `user_id` ตอน Staff/Admin ส่ง notification ใช้ `GET /api/users/:id`
- **Booking Service** — ส่ง event มาที่ `POST http://notification-service:8084/internal/events` พร้อม header `X-Internal-Key` (ไม่ใช้ token ของผู้ใช้ เพราะคนกดอนุมัติเป็น Staff แต่คนรับแจ้งเตือนคือเจ้าของการจอง) รายละเอียดและตัวอย่างโค้ดใน [EVENTS.md](EVENTS.md)
  - booking-service ต้องเพิ่ม env ใน compose: `NOTIFICATION_SERVICE_URL: http://notification-service:8084` และ `INTERNAL_API_KEY: dev-internal-key`
