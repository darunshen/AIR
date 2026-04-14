package model

import "time"

type Session struct {
	ID         string    `json:"id"`
	VMID       string    `json:"vm_id"`
	Provider   string    `json:"provider"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at"`
}
