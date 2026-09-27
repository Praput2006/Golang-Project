package main

import (
	"log"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var db *gorm.DB

func connectDB(dsn string) {
	if dsn == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	config := &gorm.Config{
		TranslateError: true,
		Logger:         logger.Default.LogMode(logger.Silent),
		// คอลัมน์เป็น TIMESTAMP (ไม่มี timezone) จึงบันทึกเป็น UTC เสมอ ไม่งั้นรันบนเครื่องเวลาไทยจะอ่านกลับมาคลาด 7 ชั่วโมง
		NowFunc: func() time.Time { return time.Now().UTC() },
	}

	// ตอนรันด้วย docker compose ฐานข้อมูลอาจยังเปิดไม่เสร็จ จึงลองซ้ำสักพักก่อนยอมแพ้
	var err error
	for attempt := 1; attempt <= 15; attempt++ {
		db, err = gorm.Open(postgres.Open(dsn), config)
		if err == nil {
			break
		}
		log.Printf("waiting for database (%d/15): %v", attempt, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("cannot connect to database: %v", err)
	}

	log.Println("connected to database")
}
