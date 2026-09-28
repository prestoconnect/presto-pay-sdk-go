package prestopay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// Operation identifies which gateway call an Error came from.
type Operation int

const (
	OpInit Operation = iota
	OpQuery
	OpReverse
	OpRefund
	OpWebhook
	OpConfig
	OpRaw
)

const (
	initPath    = "/v1/ext/payment/init"
	queryPath   = "/v1/ext/payment/query"
	reversePath = "/v1/ext/payment/reverse"
	refundPath  = "/v1/ext/payment/refund"
)

// LineItem is one entry in InitRequest.ItemList.
type LineItem struct {
	ItemDesc     string `json:"itemDesc"`
	Quantity     int64  `json:"quantity"`
	UnitAmount   int64  `json:"unitAmount"`
	TotalAmount  int64  `json:"totalAmount"`
	ImageURL     string `json:"imageUrl,omitempty"`
	ItemURL      string `json:"itemUrl,omitempty"`
	Category     string `json:"category,omitempty"`
	CategoryDesc string `json:"categoryDesc,omitempty"`
	Supplier     string `json:"supplier,omitempty"`
	SupplierDesc string `json:"supplierDesc,omitempty"`
	SupplierURL  string `json:"supplierUrl,omitempty"`
}

// RefundDetail is one entry in QueryResponse.RefundDetails.
type RefundDetail struct {
	RefundRefNum        string `json:"refundRefNum"`
	PrestoRefundRefNum  string `json:"prestoRefundRefNum"`
	RefundStatus        string `json:"refundStatus"`
	RefundRequestDate   string `json:"refundRequestDate"`
	RefundFinalisedDate string `json:"refundFinalisedDate,omitempty"`
}

// PaymentDetail is one entry in QueryResponse.PaymentDetails.
type PaymentDetail struct {
	Amount      int64  `json:"amount"`
	Method      string `json:"method,omitempty"`
	CardBin     string `json:"cardBin,omitempty"`
	CardSummary string `json:"cardSummary,omitempty"`
	CardType    string `json:"cardType,omitempty"`
	RefNum      string `json:"refNum,omitempty"`
}

// InitRequest starts a payment. PrestoMRN, TxnType, TxnRefNum and DisplayDesc
// are required; QRValue and PayerRefNum are mutually exclusive; CurrencyCode
// is required when Amount is set; RedirectURL is required when TxnType is
// TxnTypeWebPay.
type InitRequest struct {
	PrestoMRN             string     `json:"prestoMrn"`
	TxnType               string     `json:"txnType"`
	TxnRefNum             string     `json:"txnRefNum"`
	DisplayDesc           string     `json:"displayDesc"`
	QRValue               string     `json:"qrValue,omitempty"`
	PayerRefNum           string     `json:"payerRefNum,omitempty"`
	DeviceRefNum          string     `json:"deviceRefNum,omitempty"`
	DeviceIP              string     `json:"deviceIp,omitempty"`
	ItemList              []LineItem `json:"itemList,omitempty"`
	TransactionalData     string     `json:"transactionalData,omitempty"`
	Amount                int64      `json:"amount,omitempty"`
	CurrencyCode          string     `json:"currencyCode,omitempty"`
	NotifyURL             string     `json:"notifyUrl,omitempty"`
	RedirectURL           string     `json:"redirectUrl,omitempty"`
	SessionValidity       string     `json:"sessionValidity,omitempty"`
	AdditionalData        string     `json:"additionalData,omitempty"`
	Mode                  string     `json:"mode,omitempty"`
	ModeData              string     `json:"modeData,omitempty"`
	AllowedPaymentMethods []string   `json:"allowedPaymentMethods,omitempty"`
	BindData              string     `json:"bindData,omitempty"`
	ThemeRefNum           string     `json:"themeRefNum,omitempty"`
	ReceiptEmail          string     `json:"receiptEmail,omitempty"`
	ReceiptName           string     `json:"receiptName,omitempty"`
}

// InitResponse is the result of a successful Init.
type InitResponse struct {
	PrestoMRN            string `json:"prestoMrn"`
	PaymentRefNum        string `json:"paymentRefNum"`
	PaymentStatus        string `json:"paymentStatus"`
	TxnRefNum            string `json:"txnRefNum"`
	PaymentURL           string `json:"paymentUrl"`
	UserRefNum           string `json:"userRefNum,omitempty"`
	Amount               int64  `json:"amount"`
	CurrencyCode         string `json:"currencyCode,omitempty"`
	PaymentRequestDate   string `json:"paymentRequestDate,omitempty"`
	PaymentFinalisedDate string `json:"paymentFinalisedDate,omitempty"`
	AdditionalData       string `json:"additionalData,omitempty"`
}

// QueryRequest looks up a payment by PaymentRefNum or TxnRefNum (at least
// one is required), under PrestoMRN.
type QueryRequest struct {
	PrestoMRN     string `json:"prestoMrn"`
	PaymentRefNum string `json:"paymentRefNum,omitempty"`
	TxnRefNum     string `json:"txnRefNum,omitempty"`
}

// QueryResponse is the result of a successful Query.
type QueryResponse struct {
	PrestoMRN            string          `json:"prestoMrn"`
	PaymentRefNum        string          `json:"paymentRefNum"`
	TxnRefNum            string          `json:"txnRefNum,omitempty"`
	UserRefNum           string          `json:"userRefNum,omitempty"`
	PaymentStatus        string          `json:"paymentStatus,omitempty"`
	Amount               int64           `json:"amount"`
	CurrencyCode         string          `json:"currencyCode,omitempty"`
	PaymentRequestDate   string          `json:"paymentRequestDate,omitempty"`
	PaymentFinalisedDate string          `json:"paymentFinalisedDate,omitempty"`
	ReversalRefNum       string          `json:"reversalRefNum,omitempty"`
	PrestoReversalRefNum string          `json:"prestoReversalRefNum,omitempty"`
	ReversalStatus       string          `json:"reversalStatus,omitempty"`
	ReversalDate         string          `json:"reversalDate,omitempty"`
	RefundRefNum         string          `json:"refundRefNum,omitempty"`
	PrestoRefundRefNum   string          `json:"prestoRefundRefNum,omitempty"`
	RefundStatus         string          `json:"refundStatus,omitempty"`
	RefundRequestDate    string          `json:"refundRequestDate,omitempty"`
	RefundFinalisedDate  string          `json:"refundFinalisedDate,omitempty"`
	AdditionalData       string          `json:"additionalData,omitempty"`
	RefundDetails        []RefundDetail  `json:"refundDetails,omitempty"`
	PaymentDetails       []PaymentDetail `json:"paymentDetails,omitempty"`
}

// ReverseRequest reverses a payment. ReversalRefNum and one of PaymentRefNum
// / TxnRefNum are required.
type ReverseRequest struct {
	PrestoMRN      string `json:"prestoMrn"`
	ReversalRefNum string `json:"reversalRefNum"`
	PaymentRefNum  string `json:"paymentRefNum,omitempty"`
	TxnRefNum      string `json:"txnRefNum,omitempty"`
	Remark         string `json:"remark,omitempty"`
	NotifyURL      string `json:"notifyUrl,omitempty"`
}

// ReverseResponse is the result of a successful Reverse.
type ReverseResponse struct {
	PrestoMRN            string `json:"prestoMrn"`
	PaymentRefNum        string `json:"paymentRefNum"`
	PrestoReversalRefNum string `json:"prestoReversalRefNum,omitempty"`
	Amount               int64  `json:"amount"`
	CurrencyCode         string `json:"currencyCode,omitempty"`
	PaymentStatus        string `json:"paymentStatus,omitempty"`
}

// RefundRequest requests a refund. PaymentRefNum, RefundRefNum and Remark
// are required; Amount is optional and omitted for a full refund.
type RefundRequest struct {
	PrestoMRN     string `json:"prestoMrn"`
	PaymentRefNum string `json:"paymentRefNum"`
	RefundRefNum  string `json:"refundRefNum"`
	Remark        string `json:"remark"`
	NotifyURL     string `json:"notifyUrl,omitempty"`
	Amount        int64  `json:"amount,omitempty"`
}

// RefundResponse is the result of a successful Refund.
type RefundResponse struct {
	PrestoMRN          string `json:"prestoMrn"`
	PaymentRefNum      string `json:"paymentRefNum"`
	PrestoRefundRefNum string `json:"prestoRefundRefNum,omitempty"`
	Amount             int64  `json:"amount"`
	RefundAmount       int64  `json:"refundAmount,omitempty"`
	CurrencyCode       string `json:"currencyCode,omitempty"`
	PaymentStatus      string `json:"paymentStatus,omitempty"`
	RefundedDate       string `json:"refundedDate,omitempty"`
}

// PaymentsAPI groups the four payment operations under one client.
type PaymentsAPI struct {
	client *Client
}

// Init starts a payment and returns the hosted PaymentURL.
func (p PaymentsAPI) Init(ctx context.Context, req InitRequest) (*InitResponse, error) {
	if err := validateInit(req, p.client.strict); err != nil {
		return nil, err
	}
	fields, err := buildInitFields(req)
	if err != nil {
		return nil, &ConfigError{Op: OpInit, Err: err}
	}
	reconcile := &ReconcileKey{TxnRefNum: req.TxnRefNum}
	decoded, rawBody, err := p.client.send(ctx, OpInit, initPath, fields, reconcile, p.client.writePolicy())
	if err != nil {
		return nil, err
	}
	return mapInitResponse(decoded, rawBody, req, p.client.showErrorBodies, reconcile)
}

// Query reads payment state. It is safe to retry.
func (p PaymentsAPI) Query(ctx context.Context, req QueryRequest) (*QueryResponse, error) {
	if err := validateQuery(req); err != nil {
		return nil, err
	}
	fields := buildQueryFields(req)
	decoded, rawBody, err := p.client.send(ctx, OpQuery, queryPath, fields, nil, p.client.readPolicy())
	if err != nil {
		return nil, err
	}
	return mapQueryResponse(decoded, rawBody, req, p.client.showErrorBodies)
}

// Reverse reverses a payment where the gateway supports it.
func (p PaymentsAPI) Reverse(ctx context.Context, req ReverseRequest) (*ReverseResponse, error) {
	if err := validateReverse(req, p.client.strict); err != nil {
		return nil, err
	}
	fields := buildReverseFields(req)
	reconcile := &ReconcileKey{PaymentRefNum: req.PaymentRefNum}
	decoded, rawBody, err := p.client.send(ctx, OpReverse, reversePath, fields, reconcile, p.client.writePolicy())
	if err != nil {
		return nil, err
	}
	return mapReverseResponse(decoded, rawBody, req, p.client.showErrorBodies, reconcile)
}

// Refund requests a refund.
func (p PaymentsAPI) Refund(ctx context.Context, req RefundRequest) (*RefundResponse, error) {
	if err := validateRefund(req, p.client.strict); err != nil {
		return nil, err
	}
	fields := buildRefundFields(req)
	reconcile := &ReconcileKey{PaymentRefNum: req.PaymentRefNum}
	decoded, rawBody, err := p.client.send(ctx, OpRefund, refundPath, fields, reconcile, p.client.writePolicy())
	if err != nil {
		return nil, err
	}
	return mapRefundResponse(decoded, rawBody, req, p.client.showErrorBodies, reconcile)
}

// --- validation ---

func validateInit(req InitRequest, strict bool) error {
	const op = OpInit
	switch {
	case req.PrestoMRN == "":
		return &ConfigError{Op: op, Field: "PrestoMRN"}
	case req.TxnType == "":
		return &ConfigError{Op: op, Field: "TxnType"}
	case req.TxnRefNum == "":
		return &ConfigError{Op: op, Field: "TxnRefNum"}
	case req.DisplayDesc == "":
		return &ConfigError{Op: op, Field: "DisplayDesc"}
	case req.QRValue != "" && req.PayerRefNum != "":
		return &ConfigError{Op: op, Field: "PayerRefNum", Err: errors.New("mutually exclusive with QRValue")}
	case req.Amount != 0 && req.CurrencyCode == "":
		return &ConfigError{Op: op, Field: "CurrencyCode", Err: errors.New("required when Amount is set")}
	case req.TxnType == TxnTypeWebPay && req.RedirectURL == "":
		return &ConfigError{Op: op, Field: "RedirectURL", Err: errors.New("required when TxnType is WebPay")}
	}
	if !strict {
		return nil
	}
	for _, c := range []struct {
		field string
		value string
		max   int
	}{
		{"TxnRefNum", req.TxnRefNum, 50},
		{"DisplayDesc", req.DisplayDesc, 255},
		{"DeviceRefNum", req.DeviceRefNum, 50},
		{"DeviceIP", req.DeviceIP, 50},
		{"NotifyURL", req.NotifyURL, 255},
		{"RedirectURL", req.RedirectURL, 255},
		{"AdditionalData", req.AdditionalData, 255},
		{"Mode", req.Mode, 50},
		{"ModeData", req.ModeData, 1000},
		{"ReceiptEmail", req.ReceiptEmail, 320},
		{"ReceiptName", req.ReceiptName, 200},
	} {
		if err := checkMaxLen(op, c.field, c.value, c.max); err != nil {
			return err
		}
	}
	return nil
}

func validateQuery(req QueryRequest) error {
	const op = OpQuery
	if req.PrestoMRN == "" {
		return &ConfigError{Op: op, Field: "PrestoMRN"}
	}
	if req.PaymentRefNum == "" && req.TxnRefNum == "" {
		return &ConfigError{Op: op, Field: "PaymentRefNum", Err: errors.New("PaymentRefNum or TxnRefNum is required")}
	}
	return nil
}

func validateReverse(req ReverseRequest, strict bool) error {
	const op = OpReverse
	switch {
	case req.PrestoMRN == "":
		return &ConfigError{Op: op, Field: "PrestoMRN"}
	case req.ReversalRefNum == "":
		return &ConfigError{Op: op, Field: "ReversalRefNum"}
	case req.PaymentRefNum == "" && req.TxnRefNum == "":
		return &ConfigError{Op: op, Field: "PaymentRefNum", Err: errors.New("PaymentRefNum or TxnRefNum is required")}
	}
	if !strict {
		return nil
	}
	for _, c := range []struct {
		field string
		value string
		max   int
	}{
		{"ReversalRefNum", req.ReversalRefNum, 50},
		{"Remark", req.Remark, 200},
		{"NotifyURL", req.NotifyURL, 255},
	} {
		if err := checkMaxLen(op, c.field, c.value, c.max); err != nil {
			return err
		}
	}
	return nil
}

func validateRefund(req RefundRequest, strict bool) error {
	const op = OpRefund
	switch {
	case req.PrestoMRN == "":
		return &ConfigError{Op: op, Field: "PrestoMRN"}
	case req.PaymentRefNum == "":
		return &ConfigError{Op: op, Field: "PaymentRefNum"}
	case req.RefundRefNum == "":
		return &ConfigError{Op: op, Field: "RefundRefNum"}
	case req.Remark == "":
		return &ConfigError{Op: op, Field: "Remark"}
	}
	if !strict {
		return nil
	}
	for _, c := range []struct {
		field string
		value string
		max   int
	}{
		{"RefundRefNum", req.RefundRefNum, 50},
		{"Remark", req.Remark, 200},
		{"NotifyURL", req.NotifyURL, 255},
	} {
		if err := checkMaxLen(op, c.field, c.value, c.max); err != nil {
			return err
		}
	}
	return nil
}

func checkMaxLen(op Operation, field, value string, max int) error {
	if len(value) > max {
		return &ConfigError{Op: op, Field: field, Err: fmt.Errorf("exceeds the documented maximum length of %d", max)}
	}
	return nil
}

// --- building the outgoing wire body ---

func buildInitFields(req InitRequest) (map[string]any, error) {
	fields := map[string]any{
		"prestoMrn":   req.PrestoMRN,
		"txnType":     req.TxnType,
		"txnRefNum":   req.TxnRefNum,
		"displayDesc": req.DisplayDesc,
	}
	setIfNotEmpty(fields, "qrValue", req.QRValue)
	setIfNotEmpty(fields, "payerRefNum", req.PayerRefNum)
	setIfNotEmpty(fields, "deviceRefNum", req.DeviceRefNum)
	setIfNotEmpty(fields, "deviceIp", req.DeviceIP)
	if len(req.ItemList) > 0 {
		s, err := stringifyList(req.ItemList)
		if err != nil {
			return nil, fmt.Errorf("ItemList: %w", err)
		}
		fields["itemList"] = s
	}
	setIfNotEmpty(fields, "transactionalData", req.TransactionalData)
	if req.Amount != 0 {
		fields["amount"] = req.Amount
	}
	setIfNotEmpty(fields, "currencyCode", req.CurrencyCode)
	setIfNotEmpty(fields, "notifyUrl", req.NotifyURL)
	setIfNotEmpty(fields, "redirectUrl", req.RedirectURL)
	setIfNotEmpty(fields, "sessionValidity", req.SessionValidity)
	setIfNotEmpty(fields, "additionalData", req.AdditionalData)
	setIfNotEmpty(fields, "mode", req.Mode)
	setIfNotEmpty(fields, "modeData", req.ModeData)
	if len(req.AllowedPaymentMethods) > 0 {
		s, err := stringifyList(req.AllowedPaymentMethods)
		if err != nil {
			return nil, fmt.Errorf("AllowedPaymentMethods: %w", err)
		}
		fields["allowedPaymentMethods"] = s
	}
	setIfNotEmpty(fields, "bindData", req.BindData)
	setIfNotEmpty(fields, "themeRefNum", req.ThemeRefNum)
	setIfNotEmpty(fields, "receiptEmail", req.ReceiptEmail)
	setIfNotEmpty(fields, "receiptName", req.ReceiptName)
	return fields, nil
}

func buildQueryFields(req QueryRequest) map[string]any {
	fields := map[string]any{"prestoMrn": req.PrestoMRN}
	setIfNotEmpty(fields, "paymentRefNum", req.PaymentRefNum)
	setIfNotEmpty(fields, "txnRefNum", req.TxnRefNum)
	return fields
}

func buildReverseFields(req ReverseRequest) map[string]any {
	fields := map[string]any{
		"prestoMrn":      req.PrestoMRN,
		"reversalRefNum": req.ReversalRefNum,
	}
	setIfNotEmpty(fields, "paymentRefNum", req.PaymentRefNum)
	setIfNotEmpty(fields, "txnRefNum", req.TxnRefNum)
	setIfNotEmpty(fields, "remark", req.Remark)
	setIfNotEmpty(fields, "notifyUrl", req.NotifyURL)
	return fields
}

func buildRefundFields(req RefundRequest) map[string]any {
	fields := map[string]any{
		"prestoMrn":     req.PrestoMRN,
		"paymentRefNum": req.PaymentRefNum,
		"refundRefNum":  req.RefundRefNum,
		"remark":        req.Remark,
	}
	setIfNotEmpty(fields, "notifyUrl", req.NotifyURL)
	if req.Amount != 0 {
		fields["amount"] = req.Amount
	}
	return fields
}

func setIfNotEmpty(fields map[string]any, key, value string) {
	if value != "" {
		fields[key] = value
	}
}

// stringifyList renders v (a slice) as a JSON string, since list fields are
// always a JSON string containing an array on the wire, never a native
// array, in both directions.
func stringifyList(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// --- mapping the verified response body ---

// stringField reads key as a string, normalizing both an absent key and a
// JSON null to the zero value: which of the two the gateway uses for an
// empty optional field is not stable, so the two spellings are one
// condition.
func stringField(decoded map[string]any, key string) string {
	if s, ok := decoded[key].(string); ok {
		return s
	}
	return ""
}

func requireStringField(op Operation, decoded map[string]any, rawBody []byte, key string, showBody bool, reconcile *ReconcileKey) (string, error) {
	s := stringField(decoded, key)
	if s == "" {
		return "", newResponseError(op, "response", rawBody, fmt.Errorf("missing required field %q", key), showBody, reconcile)
	}
	return s, nil
}

func intField(op Operation, decoded map[string]any, rawBody []byte, key string, showBody bool, reconcile *ReconcileKey) (int64, error) {
	v, ok := decoded[key]
	if !ok || v == nil {
		return 0, nil
	}
	n, ok := v.(json.Number)
	if !ok {
		return 0, newResponseError(op, "response", rawBody, fmt.Errorf("field %q: expected a number, got %T", key, v), showBody, reconcile)
	}
	i, err := n.Int64()
	if err != nil {
		return 0, newResponseError(op, "response", rawBody, fmt.Errorf("field %q: %w", key, err), showBody, reconcile)
	}
	return i, nil
}

func checkEcho(op Operation, rawBody []byte, field, sent, got string, showBody bool, reconcile *ReconcileKey) error {
	if sent != "" && got != sent {
		return newResponseError(op, "response", rawBody,
			fmt.Errorf("%s echo mismatch: sent %q, got %q", field, sent, got), showBody, reconcile)
	}
	return nil
}

// parseListField reads a stringified-array response field into a typed
// slice; a missing or empty field reads as no elements, since whether such a
// field can be absent entirely is unconfirmed.
func parseListField[T any](op Operation, decoded map[string]any, rawBody []byte, key string, showBody bool, reconcile *ReconcileKey) ([]T, error) {
	raw, ok := decoded[key]
	if !ok || raw == nil {
		return nil, nil
	}
	s, ok := raw.(string)
	if !ok {
		return nil, newResponseError(op, "response", rawBody, fmt.Errorf("field %q: expected a JSON-string value, got %T", key, raw), showBody, reconcile)
	}
	if s == "" {
		return nil, nil
	}
	var out []T
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, newResponseError(op, "response", rawBody, fmt.Errorf("field %q: %w", key, err), showBody, reconcile)
	}
	return out, nil
}

func mapInitResponse(decoded map[string]any, rawBody []byte, req InitRequest, showBody bool, reconcile *ReconcileKey) (*InitResponse, error) {
	const op = OpInit
	paymentRefNum, err := requireStringField(op, decoded, rawBody, "paymentRefNum", showBody, reconcile)
	if err != nil {
		return nil, err
	}
	paymentStatus, err := requireStringField(op, decoded, rawBody, "paymentStatus", showBody, reconcile)
	if err != nil {
		return nil, err
	}
	prestoMrn := stringField(decoded, "prestoMrn")
	if err := checkEcho(op, rawBody, "prestoMrn", req.PrestoMRN, prestoMrn, showBody, reconcile); err != nil {
		return nil, err
	}
	txnRefNum := stringField(decoded, "txnRefNum")
	if err := checkEcho(op, rawBody, "txnRefNum", req.TxnRefNum, txnRefNum, showBody, reconcile); err != nil {
		return nil, err
	}
	amount, err := intField(op, decoded, rawBody, "amount", showBody, reconcile)
	if err != nil {
		return nil, err
	}
	return &InitResponse{
		PrestoMRN:            prestoMrn,
		PaymentRefNum:        paymentRefNum,
		PaymentStatus:        paymentStatus,
		TxnRefNum:            txnRefNum,
		PaymentURL:           stringField(decoded, "paymentUrl"),
		UserRefNum:           stringField(decoded, "userRefNum"),
		Amount:               amount,
		CurrencyCode:         stringField(decoded, "currencyCode"),
		PaymentRequestDate:   stringField(decoded, "paymentRequestDate"),
		PaymentFinalisedDate: stringField(decoded, "paymentFinalisedDate"),
		AdditionalData:       stringField(decoded, "additionalData"),
	}, nil
}

func mapQueryResponse(decoded map[string]any, rawBody []byte, req QueryRequest, showBody bool) (*QueryResponse, error) {
	const op = OpQuery
	paymentRefNum, err := requireStringField(op, decoded, rawBody, "paymentRefNum", showBody, nil)
	if err != nil {
		return nil, err
	}
	prestoMrn := stringField(decoded, "prestoMrn")
	if err := checkEcho(op, rawBody, "prestoMrn", req.PrestoMRN, prestoMrn, showBody, nil); err != nil {
		return nil, err
	}
	txnRefNum := stringField(decoded, "txnRefNum")
	if err := checkEcho(op, rawBody, "txnRefNum", req.TxnRefNum, txnRefNum, showBody, nil); err != nil {
		return nil, err
	}
	amount, err := intField(op, decoded, rawBody, "amount", showBody, nil)
	if err != nil {
		return nil, err
	}
	refundDetails, err := parseListField[RefundDetail](op, decoded, rawBody, "refundDetails", showBody, nil)
	if err != nil {
		return nil, err
	}
	paymentDetails, err := parseListField[PaymentDetail](op, decoded, rawBody, "paymentDetails", showBody, nil)
	if err != nil {
		return nil, err
	}
	return &QueryResponse{
		PrestoMRN:            prestoMrn,
		PaymentRefNum:        paymentRefNum,
		TxnRefNum:            txnRefNum,
		UserRefNum:           stringField(decoded, "userRefNum"),
		PaymentStatus:        stringField(decoded, "paymentStatus"),
		Amount:               amount,
		CurrencyCode:         stringField(decoded, "currencyCode"),
		PaymentRequestDate:   stringField(decoded, "paymentRequestDate"),
		PaymentFinalisedDate: stringField(decoded, "paymentFinalisedDate"),
		ReversalRefNum:       stringField(decoded, "reversalRefNum"),
		PrestoReversalRefNum: stringField(decoded, "prestoReversalRefNum"),
		ReversalStatus:       stringField(decoded, "reversalStatus"),
		ReversalDate:         stringField(decoded, "reversalDate"),
		RefundRefNum:         stringField(decoded, "refundRefNum"),
		PrestoRefundRefNum:   stringField(decoded, "prestoRefundRefNum"),
		RefundStatus:         stringField(decoded, "refundStatus"),
		RefundRequestDate:    stringField(decoded, "refundRequestDate"),
		RefundFinalisedDate:  stringField(decoded, "refundFinalisedDate"),
		AdditionalData:       stringField(decoded, "additionalData"),
		RefundDetails:        refundDetails,
		PaymentDetails:       paymentDetails,
	}, nil
}

func mapReverseResponse(decoded map[string]any, rawBody []byte, req ReverseRequest, showBody bool, reconcile *ReconcileKey) (*ReverseResponse, error) {
	const op = OpReverse
	paymentRefNum, err := requireStringField(op, decoded, rawBody, "paymentRefNum", showBody, reconcile)
	if err != nil {
		return nil, err
	}
	prestoMrn := stringField(decoded, "prestoMrn")
	if err := checkEcho(op, rawBody, "prestoMrn", req.PrestoMRN, prestoMrn, showBody, reconcile); err != nil {
		return nil, err
	}
	amount, err := intField(op, decoded, rawBody, "amount", showBody, reconcile)
	if err != nil {
		return nil, err
	}
	return &ReverseResponse{
		PrestoMRN:            prestoMrn,
		PaymentRefNum:        paymentRefNum,
		PrestoReversalRefNum: stringField(decoded, "prestoReversalRefNum"),
		Amount:               amount,
		CurrencyCode:         stringField(decoded, "currencyCode"),
		PaymentStatus:        stringField(decoded, "paymentStatus"),
	}, nil
}

func mapRefundResponse(decoded map[string]any, rawBody []byte, req RefundRequest, showBody bool, reconcile *ReconcileKey) (*RefundResponse, error) {
	const op = OpRefund
	paymentRefNum, err := requireStringField(op, decoded, rawBody, "paymentRefNum", showBody, reconcile)
	if err != nil {
		return nil, err
	}
	prestoMrn := stringField(decoded, "prestoMrn")
	if err := checkEcho(op, rawBody, "prestoMrn", req.PrestoMRN, prestoMrn, showBody, reconcile); err != nil {
		return nil, err
	}
	amount, err := intField(op, decoded, rawBody, "amount", showBody, reconcile)
	if err != nil {
		return nil, err
	}
	refundAmount, err := intField(op, decoded, rawBody, "refundAmount", showBody, reconcile)
	if err != nil {
		return nil, err
	}
	return &RefundResponse{
		PrestoMRN:          prestoMrn,
		PaymentRefNum:      paymentRefNum,
		PrestoRefundRefNum: stringField(decoded, "prestoRefundRefNum"),
		Amount:             amount,
		RefundAmount:       refundAmount,
		CurrencyCode:       stringField(decoded, "currencyCode"),
		PaymentStatus:      stringField(decoded, "paymentStatus"),
		RefundedDate:       stringField(decoded, "refundedDate"),
	}, nil
}
