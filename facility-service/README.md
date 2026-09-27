# Facility Service

University Facility Booking System — Facility Service (Go + Gin + GORM + PostgreSQL + Keycloak)
ผู้รับผิดชอบ: นายประพุทธ์ คำคาวี · Database: `facility_db` (`facilities`, `facility_availability`)

## Endpoints

| หมวด | Method & Path | Role | หมายเหตุ |
|---|---|---|---|
| 1. Create | `POST /api/facilities` | STAFF, ADMIN | ชื่อซ้ำ (ไม่สนตัวพิมพ์) → 409 |
| 2. List/Search | `GET /api/facilities` | ทุก role | `q`, `type`, `status`, `min_capacity`, `page`, `limit` (≤100) |
| 3. Read one | `GET /api/facilities/:id` | ทุก role | แนบ `upcoming_availability` ให้ User ดูช่วงเวลาที่ว่าง |
| 4. Update | `PUT /api/facilities/:id` | STAFF, ADMIN | partial update ส่งเฉพาะ field ที่จะแก้ |
| 5. Delete | `DELETE /api/facilities/:id` | ADMIN | ยังมีช่วงเวลาเปิดจองวันนี้/อนาคต → 409, ใส่ `?force=true` เพื่อลบทั้งหมด |
| 6. Special | `POST /api/facilities/:id/availability` | STAFF, ADMIN | ดูกฎด้านล่าง |
| 7. Report | `GET /api/reports/facilities/usage` | ADMIN | `from`, `to` (ค่าเริ่มต้น = เดือนนี้) |

**กฎของ endpoint 6:** ห้องต้อง `AVAILABLE` (ไม่งั้น 409) · วันที่ต้องไม่อยู่ในอดีต · ถ้าเป็นวันนี้ เวลาเริ่มต้องยังไม่ผ่าน · `start_time < end_time` · ห้ามทับช่วงเดิมของห้องเดียวกันในวันเดียวกัน (409) แต่ช่วงที่ต่อกันพอดี เช่น 09:00–11:00 กับ 11:00–12:00 ทำได้

**Report:** นับจาก DB ของ service ตัวเองเท่านั้น (จำนวนห้องแยกตาม type/status, ความจุรวม, จำนวนช่วงเวลาและชั่วโมงที่เปิดให้จองต่อห้อง) ตามหลัก database-per-service ถ้าต้องการจำนวนการจองจริงให้ดึงจาก `GET /api/reports/bookings/summary` ของ Booking Service

## โครงสร้าง

```
cmd/server/main.go            จุดเริ่มโปรแกรม, ต่อ DB/Keycloak (มี retry), graceful shutdown
internal/config               อ่าน env
internal/model                GORM model = ตารางตาม Database Design
internal/dto                  request/response + validation tag
internal/repository           query ฐานข้อมูล (interface + GORM)
internal/service              business rule ทั้งหมด
internal/handler              แปลง HTTP ↔ service, map error → status code
internal/middleware           Keycloak JWT (JWKS) + RequireRoles  ← ใช้ pattern เดียวกันทุก service ได้
internal/router               ผูก route + role
docs/                         OpenAPI spec + Swagger UI
deploy/                       snippet สำหรับ kong.yml และ docker-compose.yml กลาง
```

## เริ่มใช้งาน

```bash
go mod tidy                 # สร้าง go.sum
cp .env.example .env        # แล้ว export ค่า หรือกำหนดใน docker-compose
go run ./cmd/server
```

- Swagger UI: http://localhost:8082/docs  ·  Health: http://localhost:8082/health
- ต้องมี realm role `USER`, `STAFF`, `ADMIN` ใน Keycloak (ตั้งชื่อให้ตรงกันทั้งทีม)
- `KEYCLOAK_ISSUER` เว้นว่างได้ตอน dev: token ที่ขอจาก `localhost:8080` จะมี issuer ไม่ตรงกับ `keycloak:8080` ที่ service ใช้ใน docker

## Docker / Kong

ก๊อป `deploy/docker-compose.facility.yml` ไปใส่ใน `docker-compose.yml` กลาง และ `deploy/kong.facility.yml` ไปใส่ใน `kong.yml` กลาง (ใช้ `strip_path: false` เพราะ service รับ path เต็ม `/api/...`)

## Tests

```bash
go test ./...                               # unit test: service (fake repo) + router (fake auth, role check, status code)

# integration test กับ Postgres จริง (ใช้ DB แยก เพราะจะล้างตาราง)
TEST_DATABASE_URL="host=localhost user=postgres password=postgres dbname=facility_db_test port=5432 sslmode=disable" \
  go test -tags=integration ./internal/repository/...
```
