# Payment methods

This page lists the payment method codes the SDK knows. It assumes you've set up a client as in the
[quick start](../README.md#quick-start).

Payment method codes appear in two places: in `AllowedPaymentMethods`, which you can set on `InitRequest`, and
in `Method` on each `PaymentDetail` that a `QueryResponse` or a webhook event returns in `PaymentDetails`. You
only need `AllowedPaymentMethods` if you build your own payment selection page; otherwise the shopper chooses
on Presto's page.

**Which methods you can use depends on your account.** Presto enables payment methods for each merchant during
onboarding. A code being listed here doesn't make it available to you; ask Presto which methods are enabled
for your account, or to enable more.

- [Codes are plain strings](#codes-are-plain-strings)
- [Methods that need a Presto account](#methods-that-need-a-presto-account)
- [Legacy methods](#legacy-methods)
- [All payment methods](#all-payment-methods)

## Codes are plain strings

The constants are a convenience: each one's value is exactly the code Presto sends and receives, and every
field that holds a payment method is a plain string. So when Presto adds a payment method, you don't need a
new SDK release to use it. Pass its code as a string:

```go
payment, err := client.Payments.Init(ctx, prestopay.InitRequest{
    // ...
    AllowedPaymentMethods: []string{prestopay.PaymentMethodTouchNGoEWallet, "NewMethodCode"},
})
```

Like any other code, it works only once Presto has enabled it for your account.

The same applies to what you read back. `Method` is a `string`. Compare it with the constants, and give any
`switch` on it a `default` case that handles a code you don't recognise without failing: store and show it as
it is.

## Methods that need a Presto account

To pay with one of these, the shopper logs in to their Presto account on Presto's payment page. If you limit
`AllowedPaymentMethods` to these methods only, shoppers without a Presto account can't pay.

| Code | Constant | What it is |
|------|----------|------------|
| `Wallet` | `prestopay.PaymentMethodWallet` | PrestoPay eWallet |
| `CashBack` | `prestopay.PaymentMethodCashBack` | PrestoPay Credits |
| `Card` | `prestopay.PaymentMethodCard` | Tokenised card saved to the shopper's Presto account |

Loyalty programmes, where the shopper pays with points:

| Code | Constant | Programme |
|------|----------|-----------|
| `Subwallet_NearU` | `prestopay.PaymentMethodSubwallet_NearU` | NearU Points |
| `Subwallet_CARROTS` | `prestopay.PaymentMethodSubwallet_CARROTS` | Carrots |
| `Subwallet_BUDDY` | `prestopay.PaymentMethodSubwallet_BUDDY` | Buddy+ |
| `BigLife` | `prestopay.PaymentMethodBigLife` | AirAsia rewards (legacy) |
| `BonusLink` | `prestopay.PaymentMethodBonusLink` | BonusLink |
| `GOrewards` | `prestopay.PaymentMethodGOrewards` | GOrewards |
| `RISE` | `prestopay.PaymentMethodRISE` | RISE |
| `PlusMiles` | `prestopay.PaymentMethodPlusMiles` | PlusMiles |
| `VSing` | `prestopay.PaymentMethodVSing` | VSing |
| `KLEAN` | `prestopay.PaymentMethodKLEAN` | KLEAN |

`Card` and `PmPgCard` are both card payments. `Card` uses a card the shopper saved to their Presto account;
with `PmPgCard`, the shopper enters card details on Presto's payment page and doesn't need to log in.

## Legacy methods

Don't use these in new integrations:

- `TouchNGo`: use `TouchNGoEWallet` instead.
- `BigLife` (AirAsia rewards).

## All payment methods

Which of these you can use depends on what Presto enabled for your account during onboarding.

| Code | Constant | Notes |
|------|----------|-------|
| `Wallet` | `prestopay.PaymentMethodWallet` | PrestoPay eWallet. Needs a Presto account |
| `CashBack` | `prestopay.PaymentMethodCashBack` | PrestoPay Credits. Needs a Presto account |
| `Card` | `prestopay.PaymentMethodCard` | Tokenised card. Needs a Presto account |
| `BigLife` | `prestopay.PaymentMethodBigLife` | Loyalty: AirAsia rewards (legacy). Needs a Presto account |
| `BonusLink` | `prestopay.PaymentMethodBonusLink` | Loyalty points. Needs a Presto account |
| `RISE` | `prestopay.PaymentMethodRISE` | Loyalty points. Needs a Presto account |
| `PlusMiles` | `prestopay.PaymentMethodPlusMiles` | Loyalty points. Needs a Presto account |
| `VSing` | `prestopay.PaymentMethodVSing` | Loyalty points. Needs a Presto account |
| `KLEAN` | `prestopay.PaymentMethodKLEAN` | Loyalty points. Needs a Presto account |
| `GOrewards` | `prestopay.PaymentMethodGOrewards` | Loyalty points. Needs a Presto account |
| `Subwallet_NearU` | `prestopay.PaymentMethodSubwallet_NearU` | Loyalty: NearU Points. Needs a Presto account |
| `Subwallet_CARROTS` | `prestopay.PaymentMethodSubwallet_CARROTS` | Loyalty: Carrots. Needs a Presto account |
| `Subwallet_BUDDY` | `prestopay.PaymentMethodSubwallet_BUDDY` | Loyalty: Buddy+. Needs a Presto account |
| `TuneTalk` | `prestopay.PaymentMethodTuneTalk` | |
| `PmPgCard` | `prestopay.PaymentMethodPmPgCard` | Card entered on Presto's payment page |
| `Maybank` | `prestopay.PaymentMethodMaybank` | |
| `Ambank` | `prestopay.PaymentMethodAmbank` | |
| `Rhb` | `prestopay.PaymentMethodRhb` | |
| `HongLeong` | `prestopay.PaymentMethodHongLeong` | |
| `Cimb` | `prestopay.PaymentMethodCimb` | |
| `PublicBank` | `prestopay.PaymentMethodPublicBank` | |
| `AffinBank` | `prestopay.PaymentMethodAffinBank` | |
| `Bsn` | `prestopay.PaymentMethodBsn` | |
| `AllianceBank` | `prestopay.PaymentMethodAllianceBank` | |
| `AgroBank` | `prestopay.PaymentMethodAgroBank` | |
| `BankIslam` | `prestopay.PaymentMethodBankIslam` | |
| `BankOfChina` | `prestopay.PaymentMethodBankOfChina` | |
| `BankRakyat` | `prestopay.PaymentMethodBankRakyat` | |
| `BankMuamalat` | `prestopay.PaymentMethodBankMuamalat` | |
| `BoostBank` | `prestopay.PaymentMethodBoostBank` | |
| `HsbcBank` | `prestopay.PaymentMethodHsbcBank` | |
| `KuwaitFinanceHouse` | `prestopay.PaymentMethodKuwaitFinanceHouse` | |
| `OcbcBank` | `prestopay.PaymentMethodOcbcBank` | |
| `AlRajhiBank` | `prestopay.PaymentMethodAlRajhiBank` | |
| `StandardChartered` | `prestopay.PaymentMethodStandardChartered` | |
| `UobBank` | `prestopay.PaymentMethodUobBank` | |
| `MbsbBank` | `prestopay.PaymentMethodMbsbBank` | |
| `HongLeongPex` | `prestopay.PaymentMethodHongLeongPex` | |
| `UnionPay` | `prestopay.PaymentMethodUnionPay` | |
| `UnionPayQR` | `prestopay.PaymentMethodUnionPayQR` | |
| `Boost` | `prestopay.PaymentMethodBoost` | |
| `GrabPay` | `prestopay.PaymentMethodGrabPay` | |
| `GrabPayLater` | `prestopay.PaymentMethodGrabPayLater` | |
| `WeChatPayChina` | `prestopay.PaymentMethodWeChatPayChina` | |
| `TouchNGo` | `prestopay.PaymentMethodTouchNGo` | Legacy; use `TouchNGoEWallet` |
| `TouchNGoEWallet` | `prestopay.PaymentMethodTouchNGoEWallet` | Touch 'n Go eWallet |
| `AliPayChina` | `prestopay.PaymentMethodAliPayChina` | |
| `LatitudePay` | `prestopay.PaymentMethodLatitudePay` | |
| `ApplePay` | `prestopay.PaymentMethodApplePay` | |
| `GooglePay` | `prestopay.PaymentMethodGooglePay` | |
| `DuitNowQR` | `prestopay.PaymentMethodDuitNowQR` | |
