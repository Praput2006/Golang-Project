# Auth & User Service

ผู้รับผิดชอบ: นางสาวณธิดา กาฬหว้า (คนที่ 1) · พอร์ต `8081` · ฐานข้อมูล `auth_db`

## Endpoint

| หมวด | Method + Path | สิทธิ์ |
|---|---|---|
| 1. Create | `POST /api/auth/register` | ไม่ต้อง login |
| 2. List | `GET /api/users?q=&role=&status=&page=&limit=` | ADMIN |
| 3. Read own | `GET /api/users/me` | login แล้ว |
| 4. Update | `PUT /api/users/:id` | ADMIN |
| 5. Delete/Cancel | `PUT /api/users/:id/deactivate` | ADMIN |
| 6. Special | `PUT /api/users/:id/role` | ADMIN |
| 7. Report | `GET /api/reports/users/summary` | ADMIN |
| (เพิ่ม) | `GET /api/users/:id` | ADMIN, STAFF — ให้ service อื่นใช้ตรวจว่ามี user จริง |

เอกสาร Swagger: http://localhost:8081/swagger

## วิธีรัน

**รันทุกอย่างใน Docker**

```bash
docker compose up -d --build
```

**รันตอนเขียนโค้ด** (ฐานข้อมูลกับ Keycloak อยู่ใน Docker ส่วนตัว Go รันในเครื่อง)

```bash
docker compose up -d postgres keycloak
go run .
```

ค่าตั้งค่าอยู่ใน `.env` (ดูตัวอย่างที่ `.env.example`)

ตาราง `users` และ Admin เริ่มต้นสร้างจากไฟล์ `migrations/001_create_users_table.sql` โดย Postgres จะรันให้เอง**ครั้งแรกที่สร้างฐานข้อมูลเท่านั้น** ถ้าแก้ไฟล์นี้ต้องล้างฐานข้อมูลแล้วสร้างใหม่ (ข้อมูลเดิมจะหาย):

```bash
docker compose down
docker volume rm auth-service_auth-db-data
docker compose up -d
```

## ทดสอบ

```bash
go test ./...
```

unit test ทดสอบกรณีที่ตอบกลับได้โดยไม่ต้องใช้ฐานข้อมูลหรือ Keycloak คือไม่มี token (401), role ไม่พอ (403), ข้อมูลผิด (400) และการตรวจ token ของ middleware จึงรันได้ทันทีโดยไม่ต้องเปิด Docker

## บัญชีสำหรับทดสอบ (เฉพาะเครื่อง dev)

| ใช้ทำอะไร | username | password |
|---|---|---|
| Admin ของระบบ UniBook (realm `unibook`) | `uniadmin` | `UniAdmin@1234` |
| หน้า Admin Console ของ Keycloak http://localhost:8080 (realm `master`) | `admin` | `admin` |

ขอ access token (PowerShell):

```powershell
$token = (Invoke-RestMethod -Method Post -Uri http://localhost:8080/realms/unibook/protocol/openid-connect/token -Body @{grant_type="password"; client_id="unibook-frontend"; username="uniadmin"; password="UniAdmin@1234"}).access_token
Invoke-RestMethod -Uri http://localhost:8081/api/users -Headers @{Authorization="Bearer $token"}
```

## หลักการออกแบบ

- **ใช้ GORM คุยกับฐานข้อมูล** แต่โครงสร้างตารางกำหนดเองในไฟล์ SQL (`migrations/`) ให้ตรงกับเอกสาร Database Design ไม่ได้ให้ GORM สร้างตารางเอง
- **รหัสผ่านเก็บที่ Keycloak เท่านั้น** ตอนสมัคร service จะสร้างบัญชีใน Keycloak ก่อน แล้วค่อยบันทึกโปรไฟล์ลง `auth_db` โดยเก็บ `keycloak_id` ไว้เชื่อมกัน ถ้าบันทึกลงฐานข้อมูลไม่สำเร็จจะลบบัญชีใน Keycloak ทิ้ง (rollback)
- **role อ่านจาก token** (`realm_access.roles`) ทุก service ใช้ middleware แบบเดียวกัน เวลาเปลี่ยน role หรือระงับบัญชี จึงต้องแก้ที่ Keycloak ด้วยเสมอ
- role ใหม่จะมีผลเมื่อผู้ใช้ได้ token ใหม่ (token มีอายุ 15 นาที)
- ระงับบัญชีแล้วจะ login ใหม่ไม่ได้ทันที แต่ token เดิมที่ยังไม่หมดอายุยังใช้ได้จนครบ 15 นาที
