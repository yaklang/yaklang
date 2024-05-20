package yakit

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"time"
)

func SaveTrafficSession(db *gorm.DB, session *schema.TrafficSession) error {
	return db.Save(session).Error
}

func SaveTrafficPacket(db *gorm.DB, packet *schema.TrafficPacket) error {
	return db.Save(packet).Error
}

func QueryTrafficTCPReassembled(db *gorm.DB, request *ypb.QueryTrafficTCPReassembledRequest) (*bizhelper.Paginator, []*schema.TrafficTCPReassembledFrame, error) {
	db = db.Model(&schema.TrafficTCPReassembledFrame{})

	if request.GetTimestampNow() > 0 {
		db = db.Where("created_at >= ?", time.Unix(request.GetTimestampNow(), 0))
	}

	if request.GetFromId() > 0 {
		db = db.Where("id > ?", request.GetFromId())
	}

	if request.GetUntilId() > 0 {
		db = db.Where("id <= ?", request.GetUntilId())
	}

	var data []*schema.TrafficTCPReassembledFrame
	p, db := bizhelper.PagingByPagination(db, request.GetPagination(), &data)
	if db.Error != nil {
		return nil, nil, db.Error
	}
	return p, data, nil
}

func QueryTrafficSessionByUUID(db *gorm.DB, uuid string) (*schema.TrafficSession, error) {
	db = db.Model(&schema.TrafficSession{})
	db = db.Where("uuid = ?", uuid)
	var data schema.TrafficSession
	db = db.Find(&data)
	if db.Error != nil {
		return nil, db.Error
	}
	return &data, nil
}

func QueryTrafficSession(db *gorm.DB, request *ypb.QueryTrafficSessionRequest) (*bizhelper.Paginator, []*schema.TrafficSession, error) {
	db = db.Model(&schema.TrafficSession{})

	if request.GetTimestampNow() > 0 {
		db = db.Where("created_at >= ?", time.Unix(request.GetTimestampNow(), 0))
	}

	if request.GetFromId() > 0 {
		db = db.Where("id > ?", request.GetFromId())
	}

	if request.GetUntilId() > 0 {
		db = db.Where("id <= ?", request.GetUntilId())
	}

	var data []*schema.TrafficSession
	p, err := bizhelper.PagingByPagination(db, request.GetPagination(), &data)
	if err.Error != nil {
		return nil, nil, err.Error
	}
	return p, data, nil
}

func QueryTrafficPacket(db *gorm.DB, request *ypb.QueryTrafficPacketRequest) (*bizhelper.Paginator, []*schema.TrafficPacket, error) {
	db = db.Model(&schema.TrafficPacket{})
	var data []*schema.TrafficPacket

	if request.GetTimestampNow() > 0 {
		db = db.Where("created_at >= ?", time.Unix(request.GetTimestampNow(), 0))
	}

	db = db.Where("id > ?", request.GetFromId())

	p, err := bizhelper.Paging(
		db,
		int(request.GetPagination().GetPage()),
		int(request.GetPagination().GetLimit()),
		&data,
	)
	if err.Error != nil {
		return nil, nil, err.Error
	}
	return p, data, nil
}
