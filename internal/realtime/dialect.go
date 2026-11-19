package realtime

// Dialect: hinh dang JSON cua frame day xuong 1 ket noi.
//
// Mot event duoc phat ra MOT lan, nhung hai be mat API doc no khac nhau:
//
//	DialectInternal  /ws                        snake_case, nguyen struct domain
//	DialectClient    /api/client/v1/chat/ws     camelCase theo hop dong CMS,
//	                                            da loc truong noi bo
type Dialect string

const (
	DialectInternal Dialect = "internal"
	DialectClient   Dialect = "client"
)

// ClientRenderer doi payload cua event sang hinh dang cua be mat client.
// actorID = nguoi nhan frame (de loc card theo visibleTo).
//
// Tiem tu ngoai vao (xem router.go) de package realtime khong phai phu thuoc
// vao tang transport — neu import truc tiep se thanh vong:
// realtime -> clientview -> service/card -> realtime.
type ClientRenderer func(payload any, actorID string) any

// SetClientRenderer chi goi luc wiring, truoc khi hub chay.
func (h *Hub) SetClientRenderer(fn ClientRenderer) { h.clientRender = fn }
