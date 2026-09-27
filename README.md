# University Facility Booking System

ระบบจองห้องและพื้นที่ส่วนกลางของมหาวิทยาลัย — Go (Gin) Microservices + PostgreSQL + Keycloak + Kong + Docker

## โครงสร้างและผู้รับผิดชอบ

การแบ่งงานยึดตามเอกสาร **`แบ่งงานใหม่.docx`** (ฉบับแก้ไข แทนการแบ่งงานเดิมใน `project 2nd step.pdf`) เอกสารโปรเจกต์ไม่ได้เก็บใน git ขอได้ในกลุ่มแชทของทีม
ทุกคนทำ endpoint 7 หมวดเหมือนกัน (Create, List, Read, Update, Delete/Cancel, Special, Report) บวกงานส่วนกลางเท่ากัน
รายละเอียดของแต่ละคนอยู่ใน `README.md` ของโฟลเดอร์ service นั้น

| โฟลเดอร์ | Service | พอร์ต | ผู้รับผิดชอบ |
|---|---|---|---|
| `auth-service/` | Auth & User Service | 8081 | นางสาวณธิดา กาฬหว้า |
| `facility-service/` | Facility Service | 8082 | นายประพุทธ์ คำคาวี |
| `booking-service/` | Booking Service | 8083 | นายภาสกร พิจารณ์ |
| `notification-service/` | Notification Service | 8084 | นางสาววริสรา แย้มไสย |
| `keycloak/` | realm `unibook` (role USER/STAFF/ADMIN) ใช้ร่วมกัน | 8080 | ทุกคน |
| `kong/kong.yml` | route ของ API Gateway ใช้ร่วมกัน | 8000 | ทุกคนเพิ่ม route ของตัวเอง |

## รันทั้งระบบ

```bash
cp docker-compose.example.yml docker-compose.yml
docker compose up -d --build
```

## วิธีทำงานร่วมกันด้วย Git

1. ดึงงานล่าสุดก่อนเริ่มทุกครั้ง: `git pull`
2. แตก branch ของตัวเอง: `git switch -c feature/<ชื่อ-service>`
3. แก้เฉพาะในโฟลเดอร์ service ของตัวเอง (ไฟล์ส่วนกลางแก้ได้ แต่เพิ่มเฉพาะส่วนของตัวเอง)
4. commit แล้ว push branch: `git push -u origin feature/<ชื่อ-service>`
5. เปิด Pull Request เข้า `main` บน GitHub ให้เพื่อนอีกคนช่วยดูก่อน merge

ห้าม commit ไฟล์ `.env` (มีรหัสผ่าน) ให้ใช้ `.env.example` เป็นตัวอย่างแทน
