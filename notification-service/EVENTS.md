# Booking → Notification Event Contract

สำหรับ **Booking Service (คนที่ 3)** — วิธีส่ง event ให้ Notification Service สร้างแจ้งเตือนอัตโนมัติ

ไม่ใช้ message broker — Booking Service เรียก HTTP ตรงไปที่ Notification Service **ภายใน Docker network** (ไม่ผ่าน Kong)

## เมื่อไหร่ต้องส่ง

| เหตุการณ์ใน Booking Service | `event` |
|---|---|
| `POST /api/bookings` สร้างการจองสำเร็จ | `booking.created` |
| `PUT /api/bookings/:id/approve` อนุมัติสำเร็จ | `booking.approved` |
| `PUT /api/bookings/:id/reject` ปฏิเสธสำเร็จ | `booking.rejected` |

## Request

```
POST http://notification-service:8080/internal/events
Content-Type: application/json
X-Internal-Key: <ค่าเดียวกับ INTERNAL_API_KEY ของ notification-service>
```

```json
{
  "event": "booking.approved",
  "booking_id": 12,
  "user_id": 1,
  "facility_id": 3,
  "facility_name": "ห้อง 301",
  "date": "2026-10-01",
  "start_time": "09:00",
  "end_time": "11:00",
  "reject_reason": ""
}
```

| Field | จำเป็น | ความหมาย (ชื่อเดียวกับตาราง `bookings`) |
|---|---|---|
| `event` | ✅ | ดูตารางด้านบน |
| `booking_id` | ✅ | `bookings.id` |
| `user_id` | ✅ | `bookings.user_id` — **เจ้าของการจอง** (ผู้รับแจ้งเตือน) ไม่ใช่ STAFF ที่กดอนุมัติ |
| `facility_id` | - | `bookings.facility_id` |
| `facility_name` | - | ชื่อห้อง ถ้ามี (ถ้าไม่ส่ง จะแสดงเป็น "ห้อง/พื้นที่ #facility_id") |
| `date`, `start_time`, `end_time` | - | วันเวลาที่จอง (string) |
| `reject_reason` | - | `bookings.reject_reason` ใช้กับ `booking.rejected` |

## Response

| Status | ความหมาย |
|---|---|
| `201` | สร้าง notification แล้ว — `{"data": {notification}}` |
| `400` | ข้อมูลไม่ครบ / event ไม่รองรับ — `{"error": "..."}` |
| `401` | `X-Internal-Key` ไม่ถูกต้อง |

## ข้อแนะนำฝั่ง Booking Service

- ส่ง event **หลังบันทึก DB สำเร็จแล้ว** เท่านั้น
- ถ้าส่งไม่สำเร็จ (Notification ล่ม) **ไม่ต้องทำให้การจองล้มเหลว** — แค่ log error ไว้
- ตั้ง timeout สั้น ๆ (เช่น 3 วินาที)

ตัวอย่าง Go (ฝั่ง booking-service):

```go
func notifyBooking(event string, b Booking) {
	body, _ := json.Marshal(map[string]any{
		"event":         event,
		"booking_id":    b.ID,
		"user_id":       b.UserID,
		"facility_id":   b.FacilityID,
		"date":          b.Date,
		"start_time":    b.StartTime,
		"end_time":      b.EndTime,
		"reject_reason": b.RejectReason,
	})
	req, _ := http.NewRequest(http.MethodPost, os.Getenv("NOTIFICATION_SERVICE_URL")+"/internal/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Key", os.Getenv("INTERNAL_API_KEY"))

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("notify %s failed: %v", event, err)
		return
	}
	resp.Body.Close()
}
```

Environment ที่ booking-service ต้องมี (จะใส่ใน docker-compose):

```
NOTIFICATION_SERVICE_URL=http://notification-service:8080
INTERNAL_API_KEY=<ค่าเดียวกับ notification-service>
```
