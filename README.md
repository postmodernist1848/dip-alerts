# dip-alerts

Outbound-only Telegram alerts for DCA dip opportunities:

- Cifra Markets `USDT-RUB.IMEX`: −3% and −5% from the highest completed hourly close in the trailing 14 days.
- Hyperliquid spot `UBTC/USDC`: −5% and −10% from the highest completed hourly close in the trailing 7 days.

The live price is the best ask. Each tier alerts once per dip, re-arms after a 40% recovery, and resets on the DCA boundary (Sunday for BTC; the 3rd and 17th for USDT/RUB), all in `Europe/Moscow`.

## Local dry run

Create read-only Cifra API credentials, export `CIFRA_API_KEY` and `CIFRA_API_SECRET`, then run:

```bash
make check
```

This calls both data providers and prints calculations. It does not access Redis or Telegram.

## Deployment

1. Create a Telegram bot with BotFather and obtain the numeric destination chat ID.
2. Link this project to Vercel with `vercel link`.
3. Link the existing Upstash database used by `signalparser`, or copy its `KV_REST_API_URL` and `KV_REST_API_TOKEN` into this project's Vercel environment. Keys are isolated under `dip-alerts:v1:*`.
4. Add every variable from `.env.example` to the Production environment. `CIFRA_API_BASE` is optional and defaults to `https://tradernet.by/api`.
5. Deploy with `vercel --prod`.
6. In cron-job.org, create a five-minute GET job for `https://<deployment>/api/check` with header `Authorization: Bearer <SCHEDULER_SECRET>`. Enable failure notifications.

The endpoint returns HTTP 200 after an authenticated run even if one provider fails, reports that provider as an error in the JSON response, and still processes the healthy provider. There are no alternate data sources and no trading endpoints.

Transient provider errors are recorded but do not trigger Telegram messages. A failure alert is sent after 12 consecutive failed checks (about one hour with the recommended five-minute schedule), followed by a recovery alert once the provider succeeds again.

For Cifra USDT/RUB only, a stale quote last updated from 01:45 (inclusive) to 02:00 (exclusive) Moscow time suppresses outage notifications until 04:00 that same morning. This bounded rule matches an observed recurring pause; it is not a confirmed exchange maintenance schedule. Buying alerts remain skipped, the API result still includes the stale-quote error and `failureAlertSuppressed: true`, and failed checks continue to accumulate. If the quote stays stale beyond 04:00, the next check sends the usual warning once the 12-check threshold has been met. A recovery message is sent only if an outage warning was sent. Other provider errors, other quote times, previous-day frozen quotes, and Hyperliquid retain the normal failure policy.

## Development

```bash
make test
go vet ./...
```
