package prestopay

// Transaction types. The name after the prefix is the wire value verbatim.
const (
	TxnTypeQrPay      = "QrPay"
	TxnTypeWebPay     = "WebPay"
	TxnTypeMiniAppPay = "MiniAppPay"
)

// Webhook event codes. Open-ended: an unrecognized gateway value passes
// through as a plain string rather than failing.
const (
	EventCodeAuthorised = "Authorised"
	EventCodeCancelled  = "Cancelled"
	EventCodeReversed   = "Reversed"
	EventCodeRefunded   = "Refunded"
	EventCodeExpired    = "Expired"
)

// Payment statuses. Open-ended: an unrecognized gateway value passes through
// as a plain string rather than failing.
const (
	PaymentStatusPendingAuthorise = "PendingAuthorise"
	PaymentStatusCancelled        = "Cancelled"
	PaymentStatusAuthorised       = "Authorised"
	PaymentStatusFailed           = "Failed"
	PaymentStatusPendingReverse   = "PendingReverse"
	PaymentStatusReversed         = "Reversed"
	PaymentStatusPendingRefund    = "PendingRefund"
	PaymentStatusPartialRefunded  = "PartialRefunded"
	PaymentStatusRefunded         = "Refunded"
	PaymentStatusExpired          = "Expired"
)

// Reversal statuses.
const (
	ReversalStatusReversing = "Reversing"
	ReversalStatusFailed    = "Failed"
	ReversalStatusSuccess   = "Success"
)

// Refund statuses.
const (
	RefundStatusRefunding = "Refunding"
	RefundStatusFailed    = "Failed"
	RefundStatusSuccess   = "Success"
)

// Payment methods. Untyped string constants rather than a defined type: the
// list is open-ended, so a defined type would still need a default case in
// every switch and would imply a closed set that does not exist. Nobody
// "tidies" PaymentMethodPmPgCard or PaymentMethodUnionPayQR into something
// more readable — the name after the prefix is the wire value verbatim.
const (
	PaymentMethodWallet             = "Wallet"
	PaymentMethodCashBack           = "CashBack"
	PaymentMethodCard               = "Card"
	PaymentMethodBigLife            = "BigLife"
	PaymentMethodBonusLink          = "BonusLink"
	PaymentMethodRISE               = "RISE"
	PaymentMethodPlusMiles          = "PlusMiles"
	PaymentMethodVSing              = "VSing"
	PaymentMethodKLEAN              = "KLEAN"
	PaymentMethodGOrewards          = "GOrewards"
	PaymentMethodSubwallet_NearU    = "Subwallet_NearU"
	PaymentMethodSubwallet_CARROTS  = "Subwallet_CARROTS"
	PaymentMethodSubwallet_BUDDY    = "Subwallet_BUDDY"
	PaymentMethodTuneTalk           = "TuneTalk"
	PaymentMethodPmPgCard           = "PmPgCard"
	PaymentMethodMaybank            = "Maybank"
	PaymentMethodAmbank             = "Ambank"
	PaymentMethodRhb                = "Rhb"
	PaymentMethodHongLeong          = "HongLeong"
	PaymentMethodCimb               = "Cimb"
	PaymentMethodPublicBank         = "PublicBank"
	PaymentMethodAffinBank          = "AffinBank"
	PaymentMethodBsn                = "Bsn"
	PaymentMethodAllianceBank       = "AllianceBank"
	PaymentMethodAgroBank           = "AgroBank"
	PaymentMethodBankIslam          = "BankIslam"
	PaymentMethodBankOfChina        = "BankOfChina"
	PaymentMethodBankRakyat         = "BankRakyat"
	PaymentMethodBankMuamalat       = "BankMuamalat"
	PaymentMethodBoostBank          = "BoostBank"
	PaymentMethodHsbcBank           = "HsbcBank"
	PaymentMethodKuwaitFinanceHouse = "KuwaitFinanceHouse"
	PaymentMethodOcbcBank           = "OcbcBank"
	PaymentMethodAlRajhiBank        = "AlRajhiBank"
	PaymentMethodStandardChartered  = "StandardChartered"
	PaymentMethodUobBank            = "UobBank"
	PaymentMethodMbsbBank           = "MbsbBank"
	PaymentMethodHongLeongPex       = "HongLeongPex"
	PaymentMethodUnionPay           = "UnionPay"
	PaymentMethodUnionPayQR         = "UnionPayQR"
	PaymentMethodBoost              = "Boost"
	PaymentMethodGrabPay            = "GrabPay"
	PaymentMethodGrabPayLater       = "GrabPayLater"
	PaymentMethodWeChatPayChina     = "WeChatPayChina"
	PaymentMethodTouchNGo           = "TouchNGo"
	PaymentMethodTouchNGoEWallet    = "TouchNGoEWallet"
	PaymentMethodAliPayChina        = "AliPayChina"
	PaymentMethodLatitudePay        = "LatitudePay"
	PaymentMethodApplePay           = "ApplePay"
	PaymentMethodGooglePay          = "GooglePay"
	PaymentMethodDuitNowQR          = "DuitNowQR"
)

// Gateway error codes, four-digit strings. Descriptive names map to the
// codes because "1203" is not a name; unknown codes are passed through as
// plain strings rather than failing.
const (
	// Request and auth.
	ErrorCodeInvalidRequestPath          = "1001"
	ErrorCodeInvalidContentType          = "1002"
	ErrorCodeMissingMasterMerchantRef    = "1003"
	ErrorCodeInvalidTimestampFormat      = "1004"
	ErrorCodeClockSkew                   = "1005"
	ErrorCodeInvalidSignature            = "1006"
	ErrorCodeSignatureVerificationFailed = "1007"
	ErrorCodeMissingAuthorizationHeader  = "1008"
	ErrorCodeInvalidAuthorizationHeader  = "1009"
	ErrorCodeInvalidAccessToken          = "1010"
	ErrorCodeAccessTokenValidationError  = "1011"
	ErrorCodeInvalidAccessTokenOwnership = "1012"
	ErrorCodeOAuthResourceConfigError    = "1013"
	ErrorCodeMissingOAuthScope           = "1014"
	ErrorCodeOAuthServiceUnavailable     = "1015"

	// Merchant.
	ErrorCodeMerchantGeneralError                 = "1100"
	ErrorCodeMerchantInvalidInput                 = "1101"
	ErrorCodeInvalidMID                           = "1102"
	ErrorCodeMasterMerchantInfoRetrievalFailed    = "1103"
	ErrorCodeMasterMerchantInfoServiceUnavailable = "1104"
	ErrorCodeOnboardProcessingFailed              = "1105"
	ErrorCodeInvalidMerchantReference             = "1106"
	ErrorCodeDocumentUploadFailed                 = "1107"
	ErrorCodeMissingDocument                      = "1108"
	ErrorCodeRecordExists                         = "1109"
	ErrorCodeProfileNotFound                      = "1110"
	ErrorCodeInvalidMerchantTxnType               = "1111"
	ErrorCodeFileRetryLimitExceeded               = "1112"
	ErrorCodeMerchantRejected                     = "1113"

	// Payment.
	ErrorCodePaymentGeneralError            = "1200"
	ErrorCodeInvalidInput                   = "1201"
	ErrorCodeInitFailed                     = "1202"
	ErrorCodeDuplicateTxnRefNum             = "1203"
	ErrorCodeQRValueNotRecognised           = "1204"
	ErrorCodeQRValueNotBound                = "1205"
	ErrorCodeQRTOTPExpired                  = "1206"
	ErrorCodeQRValidationFailed             = "1207"
	ErrorCodeQRInvalidTOTPSecret            = "1208"
	ErrorCodeQRInvalidTOTP                  = "1209"
	ErrorCodeQRInvalidPrefix                = "1210"
	ErrorCodeQRPaymentSuspended             = "1211"
	ErrorCodePaymentNotFound                = "1212"
	ErrorCodeInvalidStatusForAuthorisation  = "1213"
	ErrorCodeAuthorisationGeneralError      = "1214"
	ErrorCodeQRValueAlreadyUsed             = "1215"
	ErrorCodeInvalidUser                    = "1216"
	ErrorCodeQueryAccessDenied              = "1217"
	ErrorCodeQueryFailed                    = "1218"
	ErrorCodeInvalidStatusForReversal       = "1219"
	ErrorCodeReversalNotAllowedSettled      = "1220"
	ErrorCodeReversalGracePeriodEnded       = "1221"
	ErrorCodeRefundToAccountFailed          = "1222"
	ErrorCodeReversalFailed                 = "1223"
	ErrorCodeReversalAlreadyInProgress      = "1224"
	ErrorCodeInvalidPaymentMethod           = "1225"
	ErrorCodeRefundAlreadyInProgress        = "1226"
	ErrorCodeInvalidStatusForRefund         = "1227"
	ErrorCodeRefundGracePeriodEnded         = "1228"
	ErrorCodeInsufficientUnsettledAmount    = "1229"
	ErrorCodeRefundFailed                   = "1230"
	ErrorCodePaymentLimitExceeded           = "1231"
	ErrorCodeInvalidSessionValidityPeriod   = "1232"
	ErrorCodeInvalidSessionValidityFormat   = "1233"
	ErrorCodePaymentMethodMismatch          = "1234"
	ErrorCodeRefundAmountExceedsTransaction = "1235"
	ErrorCodeRefundableAmountExceeded       = "1236"

	// Captured directly from a real staging response rather than the
	// documented list, which jumps from 1236 to 1400: reversing or
	// refunding a guest-checkout payment fails with this code.
	ErrorCodeGuestAccountRefundNotSupported = "1242"

	// User.
	ErrorCodeUserGeneralError             = "1400"
	ErrorCodeInvalidUserTokenFormat       = "1401"
	ErrorCodeUserTokenNotFound            = "1402"
	ErrorCodeUserReferenceRetrievalFailed = "1403"
	ErrorCodeUserProfileRetrievalFailed   = "1404"
	ErrorCodeUserInfoRetrievalFailed      = "1405"
)
