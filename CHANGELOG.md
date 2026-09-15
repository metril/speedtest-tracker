# Changelog

## [0.8.0](https://github.com/metril/speedtest-tracker/compare/v0.7.1...v0.8.0) (2026-09-15)


### ⚠ BREAKING CHANGES

* **docker:** the container user changed from uid 10001 to 1000:1000. Data volumes created by earlier images need a one-time `chown -R 1000:1000` on their contents.

### Features

* **docker:** run as uid/gid 1000 instead of 10001 ([8b6eb33](https://github.com/metril/speedtest-tracker/commit/8b6eb3394cecbbcd6ac5347d25d5bbc6d1a24512))

## [0.7.1](https://github.com/metril/speedtest-tracker/compare/v0.7.0...v0.7.1) (2026-09-15)


### Features

* **web:** view run errors in a dialog ([5b33f95](https://github.com/metril/speedtest-tracker/commit/5b33f95940c594298a1d7b02fe2f071e5b025a83))


### Bug Fixes

* **web:** settings tab a11y/history, error dialog copy, stale queue id ([e4a0636](https://github.com/metril/speedtest-tracker/commit/e4a063638e9187d2716d166e043d93b86baf29d2))

## [0.7.0](https://github.com/metril/speedtest-tracker/compare/v0.6.0...v0.7.0) (2026-09-15)


### ⚠ BREAKING CHANGES

* **notify:** the standalone "ntfy" channel type is removed. Any existing ntfy channel is rewritten to type "apprise" automatically on first startup after upgrading (an ntfy://[token@]host/topic?priority=&tags= URL built from its old fields); a channel whose old URL can't be parsed is disabled, not deleted, and a warning is logged. The external Apprise API server this app used to require is no longer used at all.

### Features

* **notify:** embed apprise-go, drop ntfy channel type and external Apprise server ([d80111e](https://github.com/metril/speedtest-tracker/commit/d80111e899ecc9f08aa2881b4dd59a096035fc0e))
* rotate through an ordered host list per target run ([4a01a53](https://github.com/metril/speedtest-tracker/commit/4a01a530e6ece85a792d6f4802071ddf703c45a2))
* user-managed queues replace fixed wan/lan lanes ([a42b6dd](https://github.com/metril/speedtest-tracker/commit/a42b6dd325051a8b06c24c3935a665a1c97f9548))
* **web:** add Queues page, rename Lane to Queue throughout ([41e9b24](https://github.com/metril/speedtest-tracker/commit/41e9b24f6e11aa5b8396ae999584426718380a70))
* **web:** rotate through an ordered host/server list per target ([bb4b797](https://github.com/metril/speedtest-tracker/commit/bb4b797d7269b5ed7d7d08f35e1b11fc206ed045))
* **web:** surface speed test errors in live panel, results and dashboard ([5cabceb](https://github.com/metril/speedtest-tracker/commit/5cabceb9a4856297940f394935df893546f8193d))


### Bug Fixes

* harden queue resolution, key runner channels by id, address review findings ([70a0772](https://github.com/metril/speedtest-tracker/commit/70a0772fe60584bb3b690a0312b51c8c9cf967dd))
* migration 0007 fails against a pre-queues db with foreign_keys=ON ([3207cf5](https://github.com/metril/speedtest-tracker/commit/3207cf5a407010fe36f671036897598a939274e0))
* **notify:** apprise ntfy migration bearer auth, dead priority validation, inert apprise tags UI ([b7215e8](https://github.com/metril/speedtest-tracker/commit/b7215e82bfcd167effbf805718485acc1aa51d59))
* redact apprise credentials, match masked URLs by identity, accept legacy lane alias ([417f599](https://github.com/metril/speedtest-tracker/commit/417f59930b95d6ffb3775003301e4b677d75ff31))
* RedactURL keeps only scheme://host, collapses path+query to /*** ([9548a3a](https://github.com/metril/speedtest-tracker/commit/9548a3abeea048293254c304012c8b7c6d2bdb16))
* **web:** align previous-period overlay by time offset ([a9e6872](https://github.com/metril/speedtest-tracker/commit/a9e68727b20ee21574b266b6b358b8d8e79e7c84))
* **web:** dismiss live drawer for user-started runs ([85dc69d](https://github.com/metril/speedtest-tracker/commit/85dc69dd836453b5809188c8726d986c2bb62f92))
* **web:** pass run id on re-execute open, fix CompactBar expand arg ([6a28ad1](https://github.com/metril/speedtest-tracker/commit/6a28ad13a8692f4c5da507b67196ea45dc438092))
* **web:** pick up engine error from result event in live run state ([8cdd19f](https://github.com/metril/speedtest-tracker/commit/8cdd19f8a0b03a76e9278c93b98e55db25915e2f))

## [0.6.0](https://github.com/metril/speedtest-tracker/compare/v0.5.0...v0.6.0) (2026-09-14)


### Features

* per-target Custom notification gate with per-metric disable ([6f5bc54](https://github.com/metril/speedtest-tracker/commit/6f5bc5462c2965bc2431e9cc2212aa6cd167c1a7))
* **web:** cloudflare size presets with a Custom sizes toggle ([1c7cb91](https://github.com/metril/speedtest-tracker/commit/1c7cb917d293294689ddd94620f010afd31797b2))

## [0.5.0](https://github.com/metril/speedtest-tracker/compare/v0.4.0...v0.5.0) (2026-09-14)


### Features

* **web:** replace iperf3 advanced disclosure with a Custom toggle ([47b3241](https://github.com/metril/speedtest-tracker/commit/47b32418c232167827c6fd52acaf17072fda344a))
* **web:** share a TimezoneSelect between General settings and schedules ([0ca8cb2](https://github.com/metril/speedtest-tracker/commit/0ca8cb233e8746cb721a01a4ff792478de51ed57))


### Bug Fixes

* **web:** drop fake engine from user-facing dropdowns ([9795de6](https://github.com/metril/speedtest-tracker/commit/9795de67045cae813e165310707b3d1f9cf5b3c0))
* **web:** fall back off crypto.randomUUID for non-secure origins ([2971dcf](https://github.com/metril/speedtest-tracker/commit/2971dcf2670e6b514791c13f40471ef087567a61))
* **web:** review follow-ups for form/button consistency ([7bae073](https://github.com/metril/speedtest-tracker/commit/7bae073233c5af034074a30d7c90bce9c1396cc3))

## [0.4.0](https://github.com/metril/speedtest-tracker/compare/v0.3.0...v0.4.0) (2026-09-14)


### Features

* compare-with-previous-period overlay ([b8f6903](https://github.com/metril/speedtest-tracker/commit/b8f690302f9d327683f54c2f78a3933bffcafcbf))
* **engine:** iperf3 port-range retry on busy server ([35b1d44](https://github.com/metril/speedtest-tracker/commit/35b1d44bc0ca2f21727ed218b6116eff77d77b5d))
* postcode geocoding via Nominatim with country hint ([2d14cea](https://github.com/metril/speedtest-tracker/commit/2d14cea114e32024be21bf5659c3613d162cd5d0))
* richer iperf3 public list ([e81e7d0](https://github.com/metril/speedtest-tracker/commit/e81e7d004381137f5a0bc986f2e7ee1c2194fab9))
* SLA compliance ([4533b51](https://github.com/metril/speedtest-tracker/commit/4533b5140a5e07352243d67e20949729fe4d4396))
* **web:** compare-with-previous-period overlay ([15ab66e](https://github.com/metril/speedtest-tracker/commit/15ab66ea0b012fe47d0fd3315793a83c8cb72b4f))
* **web:** iperf3 advanced options disclosure ([2654caf](https://github.com/metril/speedtest-tracker/commit/2654cafbd755b85fd485da4cc2c055a5325466dc))
* **web:** postcode geocoding search with country hint ([1241c0f](https://github.com/metril/speedtest-tracker/commit/1241c0ffd922f7597c94c69c83092bc368a1a656))
* **web:** richer iperf3 public list ([c5ed6d1](https://github.com/metril/speedtest-tracker/commit/c5ed6d1fe74e3f6964a870931f020c19d7b1cccd))
* **web:** SLA compliance ([1b72547](https://github.com/metril/speedtest-tracker/commit/1b72547315efaa4b69a5648e78c9e9d35e71bd58))
* **web:** Switch/Checkbox primitives, replace all native checkboxes ([0f0e2a5](https://github.com/metril/speedtest-tracker/commit/0f0e2a565ed8ebf921d7aae07baf396a5e4fdaa8))


### Bug Fixes

* **ooklaweb:** re-search geocoded postcodes by coordinates ([82c6e28](https://github.com/metril/speedtest-tracker/commit/82c6e287f96c29fb5fa1e768686d6064c19034dc))
* single connecting event per iperf3 attempt, reject negative history offset ([d89a09a](https://github.com/metril/speedtest-tracker/commit/d89a09abac9527578465b49ed8c6c2b6c2da53dc))
* SLA plan edge cases and review follow-ups ([4980f51](https://github.com/metril/speedtest-tracker/commit/4980f5124f6fdfbb5581de31649364c17530b8d9))
* **web:** popover width + empty state ([2df580f](https://github.com/metril/speedtest-tracker/commit/2df580fd0ad4bf42944659a37e04042bbd267efa))
* **web:** review follow-ups for picker, country input and SLA fields ([a6a3907](https://github.com/metril/speedtest-tracker/commit/a6a3907de98dd705897ddf6ff970cfa21bd3875f))
* **web:** review follow-ups for target form and settings nav ([e17cc22](https://github.com/metril/speedtest-tracker/commit/e17cc2259962ec4806bb3b8b51bfce96e96c3a90))
* **web:** theme toggle compacts with sidebar ([1a6cf46](https://github.com/metril/speedtest-tracker/commit/1a6cf46ded423504eb0ff2803e21ab48e330bd57))

## [0.3.0](https://github.com/metril/speedtest-tracker/compare/v0.2.0...v0.3.0) (2026-09-14)


### Features

* **api:** rate-limit ookla search, configurable iperf3 list URL, escape LIKE ([3ec64de](https://github.com/metril/speedtest-tracker/commit/3ec64de728c1fa551cba2c9e9945da61a6b5b84a))
* **api:** summary offset for previous-period comparison ([cd31114](https://github.com/metril/speedtest-tracker/commit/cd31114dd41e4d0b82f84447849a161ecde18546))
* **iperf3:** import the public iperf3 server list with a picker and daily refresh ([2f29ec8](https://github.com/metril/speedtest-tracker/commit/2f29ec84a1d523533e3f47a4ed0e38ea56c79ca1))
* **ookla:** search speedtest.net and geocode postcodes for server picker ([3a58e3b](https://github.com/metril/speedtest-tracker/commit/3a58e3bb353e502fa6eeb69f0bc55df2e175c9b9))
* **targets:** keep a revision history with revert and restore ([dfbf631](https://github.com/metril/speedtest-tracker/commit/dfbf631966d606f22e85ba037d0a4a0a00ca7073))
* **web:** add shadcn/ui primitives on existing theme tokens ([a03f0ca](https://github.com/metril/speedtest-tracker/commit/a03f0caee0759b367fcd87ca588444c9b6aca8ba))
* **web:** searchable, sortable target picker for schedules ([8e91155](https://github.com/metril/speedtest-tracker/commit/8e9115509f093e847917a30f37724243f8d2b1d1))
* **web:** sidebar layout and redesigned dashboard ([d388a93](https://github.com/metril/speedtest-tracker/commit/d388a93a5d10deac1c7ef7b007e89058a1736777))


### Bug Fixes

* **api,web:** decouple ookla local/remote search, unstick the picker ([c2ff761](https://github.com/metril/speedtest-tracker/commit/c2ff7612a22a954ccb7984f6a1c1384324e4b333))
* **api:** kick iperf3 refresher live on settings change, recover singleflight panics ([ddcd2a2](https://github.com/metril/speedtest-tracker/commit/ddcd2a22aa5114f75e220bddfe11959beadaa6c1))
* **api:** make previous-period summary window half-open at the boundary ([680f61b](https://github.com/metril/speedtest-tracker/commit/680f61b835e28495f6bde34344ba6f8dca17fa38))
* cap outbound response bodies and tighten dashboard/restore details ([49d36af](https://github.com/metril/speedtest-tracker/commit/49d36afd36b435f99a8c6ab556b67d5da0b7f17e))
* **ooklaweb:** sort coordinate-less servers last, log geocode failures ([3847200](https://github.com/metril/speedtest-tracker/commit/3847200069f8c9dbc18450437e8e17589e0f2cbe))
* **targets:** invalidate deleted-targets on delete, keep restore's created_at ([058ec7a](https://github.com/metril/speedtest-tracker/commit/058ec7a371f374adaf6a106a476ba0294efef1d2))
* **web:** position virtualized rows with top so drag commits, finish Settings ui migration ([130ba83](https://github.com/metril/speedtest-tracker/commit/130ba8397f07a301b3dd20e2b221d04b10f3f8eb))
* **web:** stop shadcn surface tokens from shadowing text-accent/text-muted ([f3e1afc](https://github.com/metril/speedtest-tracker/commit/f3e1afcfd1460cadc63d54f3d61531ac39b6890f))
* **web:** virtualize SortableTargetList as li&gt;ol with drag, gate iperf3 picker query ([5f146c9](https://github.com/metril/speedtest-tracker/commit/5f146c9295020d12d762d8e3a5876d1b7beec37c))

## [0.2.0](https://github.com/metril/speedtest-tracker/compare/v0.1.0...v0.2.0) (2026-09-14)


### Features

* **api:** add bucketed target history and outage endpoints ([117b478](https://github.com/metril/speedtest-tracker/commit/117b478216a280a153812fbcb36f22169ab09212))
* **api:** add chi router with healthz, compression and slog logging ([ded3e89](https://github.com/metril/speedtest-tracker/commit/ded3e89421091ef7c392654a634e31cb60cce00e))
* **api:** API token create, list and revoke endpoints ([d4f0848](https://github.com/metril/speedtest-tracker/commit/d4f0848be2a0aed013decac5a5ffb8f97774a472))
* **api:** auth settings section with locked keys and a lockout guard ([cc30a11](https://github.com/metril/speedtest-tracker/commit/cc30a117380dc896ad32a00ccd67a5a3d56fc264))
* **api:** connection test endpoint for VM and VL ([fc71634](https://github.com/metril/speedtest-tracker/commit/fc71634591e3082c9cec438e5350fe4b82c0018a))
* **api:** gate the v1 tree behind auth and add GET /api/v1/me ([6636166](https://github.com/metril/speedtest-tracker/commit/66361668c5aae757694b94ba24081c9add87b541))
* **api:** JSON error envelope, security headers and the SSE events endpoint ([d5d98fe](https://github.com/metril/speedtest-tracker/commit/d5d98fe2bfb4d4ab3efedafdf25d9347174a5e26))
* **api:** notifications settings section and channel test endpoint ([46cf403](https://github.com/metril/speedtest-tracker/commit/46cf40333a658870fa8ec714879a068309e8f5b9))
* **api:** report next_run from the running scheduler and last_run per schedule ([6e8294a](https://github.com/metril/speedtest-tracker/commit/6e8294a96a08d59dc5bf884209a8b2c1eb782f21))
* **api:** run control, result listing with cursor, re-execute and tags ([d6494e5](https://github.com/metril/speedtest-tracker/commit/d6494e5970050b4c36b95ccbdf9abaf30148d019))
* **api:** schedule run-now, next fire times, overlap warnings and run filter ([5f8af26](https://github.com/metril/speedtest-tracker/commit/5f8af26f98a7ad9419e13783e40613e87828b03d))
* **api:** schedules CRUD with cron and timezone validation ([4a87612](https://github.com/metril/speedtest-tracker/commit/4a87612de90b7223b03022c92e848df5da6a7b05))
* **api:** sectioned settings endpoint with masked secrets ([fea3cd1](https://github.com/metril/speedtest-tracker/commit/fea3cd17d719a25bec3310ac99fb08c54cf79566))
* **api:** stream a CSV results export and add tag rename/delete ([ac48f18](https://github.com/metril/speedtest-tracker/commit/ac48f18fda3197817110177544e4bb5017e451a1))
* **api:** target CRUD, manual run, latest result and Ookla server search ([c432e26](https://github.com/metril/speedtest-tracker/commit/c432e26648be73958999197d1479133641a35deb))
* **auth:** forward-auth and API token identity middleware ([6315af7](https://github.com/metril/speedtest-tracker/commit/6315af7dee125b831dd848eade765b39139bb3e1))
* **cli:** add run subcommand for one-off engine execution ([2db9d00](https://github.com/metril/speedtest-tracker/commit/2db9d00ea775b984a1b8cb44fd4c2433a4f92a7c))
* **cloudflare:** add native Cloudflare speed-test engine ([2b6d7e0](https://github.com/metril/speedtest-tracker/commit/2b6d7e01895e62c2369fe45b6dc6d7d6030ba828))
* **cmd:** run the cron scheduler and reload it on schedule and timezone changes ([fd7835e](https://github.com/metril/speedtest-tracker/commit/fd7835e4ee1eaa2340574a5f2755c494f9f1129a))
* **cmd:** wire auth middleware, env seeding and a joined settings watcher ([0b16a05](https://github.com/metril/speedtest-tracker/commit/0b16a057438174140c65f3b73e5da4bd5a826856))
* **cmd:** wire config, store, settings, api and embedded UI ([7130545](https://github.com/metril/speedtest-tracker/commit/713054572d7ab7458bf35eebce56429c879afe02))
* **cmd:** wire metrics, VM/VL clients and retention pruning ([4bae61f](https://github.com/metril/speedtest-tracker/commit/4bae61f38cb81426e70d82e1c6ce4779a96c03e4))
* **cmd:** wire runner, SSE hub and live settings reload into the server ([9aa3ecf](https://github.com/metril/speedtest-tracker/commit/9aa3ecf705496e4150b503b8aacf9789e2e040d9))
* **cmd:** wire the notifier as a result sink with live reload ([506a55a](https://github.com/metril/speedtest-tracker/commit/506a55a36ab9fe608b9acf3687e6fad50fac99a5))
* **config:** add bootstrap env configuration ([848dfc7](https://github.com/metril/speedtest-tracker/commit/848dfc7a0c9464ebc63883da2571ad48116c62c8))
* **engine:** add deterministic fake engine ([46d34c2](https://github.com/metril/speedtest-tracker/commit/46d34c21a1d45be22f803ecaf6ba8fd30c0bac7e))
* **engine:** add Engine interface, Result, Progress and Registry ([f13a784](https://github.com/metril/speedtest-tracker/commit/f13a78450b4d697a1be9c2ee4e3f0fb7306e5d5a))
* **iperf3:** add options validation, argument builder and version probe ([02a11d0](https://github.com/metril/speedtest-tracker/commit/02a11d0ae86756d4554306efd35a03c8658c8028))
* **iperf3:** parse summary and json-stream output into Result ([6da49c0](https://github.com/metril/speedtest-tracker/commit/6da49c0f5ff12362ec183e7c59e818cd6ee2695b))
* **metrics:** Prometheus registry and gated /metrics endpoint ([1e488ba](https://github.com/metril/speedtest-tracker/commit/1e488bae5c213f1e40fdc6610cefafa8f27d3ba1))
* **notify:** async notifier with cooldown, quiet hours and recovery ([05058a5](https://github.com/metril/speedtest-tracker/commit/05058a507b66f88efa4c1ab20a0f4471740aa895))
* **notify:** threshold merge and per-metric evaluation ([1f8ca6f](https://github.com/metril/speedtest-tracker/commit/1f8ca6f0c21a7929cb82d05b67b7071d6601bf52))
* **notify:** webhook, ntfy and apprise delivery ([3614251](https://github.com/metril/speedtest-tracker/commit/3614251939850686fae4ad2f305c1b602d13873a))
* **ookla:** add server list fetch with TTL cache ([cad3dec](https://github.com/metril/speedtest-tracker/commit/cad3dec6115ef6ad27f7ae7182138afaea5fee28))
* **ookla:** parse speedtest CLI jsonl stream into Result and Progress ([3606e2e](https://github.com/metril/speedtest-tracker/commit/3606e2e870edb37dcb380bf00e65523e5e992868))
* **ookla:** run the speedtest CLI and stream progress ([fa8880c](https://github.com/metril/speedtest-tracker/commit/fa8880c7455ea215b6c8f7ca08fc9ef936bf29d2))
* **prune:** retention pruning for results and runs ([116ac5b](https://github.com/metril/speedtest-tracker/commit/116ac5b313147ca5c589d70c45d533e4a5637733))
* **runner:** emit per-target stepper counts on run events ([01fbe69](https://github.com/metril/speedtest-tracker/commit/01fbe69c1ef97e4a2180dcf464dca85ae8599107))
* **runner:** fan out persisted results to result sinks ([7b1fab7](https://github.com/metril/speedtest-tracker/commit/7b1fab7bc01114cc551f932476535723fc9d2df3))
* **runner:** per-lane queues, async runs, progress coalescing and cancel ([588a082](https://github.com/metril/speedtest-tracker/commit/588a0828941a79dd03fd3a94d4b3cfb6abda3d31))
* **scheduler:** cron entries that enqueue runs and record skipped fires ([339dc02](https://github.com/metril/speedtest-tracker/commit/339dc02f415982abd73842ff68a64ba284e9256c))
* **scheduler:** cron expression helpers and lane overlap detection ([cdfe295](https://github.com/metril/speedtest-tracker/commit/cdfe295d404ad8bf8cfb5b46d689f8ec854e5434))
* **settings:** add auth section with modes, headers and trusted proxies ([e9d431a](https://github.com/metril/speedtest-tracker/commit/e9d431aa66ab28c512588d42d6dc825c9842abb0))
* **settings:** add Engines section with binary paths and defaults ([5ca88a3](https://github.com/metril/speedtest-tracker/commit/5ca88a31d55422b4961fd9c0c4989dc0e3e527d1))
* **settings:** add Integrations section and prune interval key ([d5cbfb1](https://github.com/metril/speedtest-tracker/commit/d5cbfb13976ba082f7698ce8688a12dae8bbb481))
* **settings:** add notifications section with channels and thresholds ([b42ced0](https://github.com/metril/speedtest-tracker/commit/b42ced0a8e3d4d34054b988affafebc864661cbf))
* **settings:** add typed settings store with subscriptions ([28d0057](https://github.com/metril/speedtest-tracker/commit/28d00572763476a2094d955489df7617fc9ef3c3))
* **settings:** seed settings from ST_ environment variables with optional locking ([e683a69](https://github.com/metril/speedtest-tracker/commit/e683a69d4005120db1cbcf0ffdd8b9d7915c4e75))
* **sse:** non-blocking event hub; make engine registry concurrency-safe ([0877fad](https://github.com/metril/speedtest-tracker/commit/0877fad4cf6b475e42a92ac462331e0e214cc7b6))
* **store:** add per-target and overall summary aggregates ([b06b982](https://github.com/metril/speedtest-tracker/commit/b06b982e89e4a07f4e28ec545899690bc835510e))
* **store:** add SQL-downsampled history buckets ([371a5cc](https://github.com/metril/speedtest-tracker/commit/371a5cc5df7a55a0ffc27d8c5e13cb099fa253f2))
* **store:** add SQLite store with pools and embedded migrations ([21c5172](https://github.com/metril/speedtest-tracker/commit/21c5172933a893d066ed877c3f6f2e32db035e76))
* **store:** api_tokens table with hashed lookup and usage tracking ([9910c76](https://github.com/metril/speedtest-tracker/commit/9910c762049311076db6222cc6ddbfdcd8fde704))
* **store:** batch schedule last runs, add tag rename/delete and a streaming result iterator ([fe3c439](https://github.com/metril/speedtest-tracker/commit/fe3c439a7fa74802ac7586f3d6e474b4da2634f4))
* **store:** group failures and skipped runs into outage incidents ([716a938](https://github.com/metril/speedtest-tracker/commit/716a938d389e52a216ca9b076b15d5c5fafd3cc7))
* **store:** notification_state upsert, lookup and clear ([2dcb990](https://github.com/metril/speedtest-tracker/commit/2dcb990d94e8c432f86a911bd653fe3d69bce5d5))
* **store:** result insert, keyset listing, filters and latest-per-target ([10d4cfe](https://github.com/metril/speedtest-tracker/commit/10d4cfeb562f3ec6e580e530585b29bfda77697c))
* **store:** result tags with set-replace semantics ([45f72f6](https://github.com/metril/speedtest-tracker/commit/45f72f67ea6f1c0ae4fafd81d02d9ce09822f094))
* **store:** run creation, status transitions and keyset listing ([c236688](https://github.com/metril/speedtest-tracker/commit/c2366882ffdff632bf1492be865082bc8aab89ec))
* **store:** schedules CRUD, ordered targets, skipped runs and run filter ([f0fd383](https://github.com/metril/speedtest-tracker/commit/f0fd383c610223158f340a1b49af33c47752961f))
* **store:** typed target CRUD queries ([60514c6](https://github.com/metril/speedtest-tracker/commit/60514c64fb947f1878b24fb75c08f2e5d7125996))
* **vlpush:** batching slog handler for VictoriaLogs ([79553a3](https://github.com/metril/speedtest-tracker/commit/79553a3d646ebfa29d1ff2346c5a69a4bc06a63c))
* **vmpush:** VictoriaMetrics writer with bounded retry ring ([04419eb](https://github.com/metril/speedtest-tracker/commit/04419ebaca962c338196295dd908f7d964e7ef23))
* **web:** add a light/dark/system theme with semantic colour tokens ([f420234](https://github.com/metril/speedtest-tracker/commit/f420234d90f8fb158e2505ec77c6c84c972ab831))
* **web:** add CSV export, tag management and batched schedule last runs ([0fe700a](https://github.com/metril/speedtest-tracker/commit/0fe700ae29a8ab93c88b77b14327ae3b881f26e6))
* **web:** add dashboard summary tiles, target cards and the range picker ([b747c34](https://github.com/metril/speedtest-tracker/commit/b747c3490c0f876ba5c5453c04229ddd4e8b6b5e))
* **web:** add Vite React TypeScript app shell ([df340ee](https://github.com/metril/speedtest-tracker/commit/df340eef949e1d46fa0007daa69dd1019261c1ba))
* **web:** api client and hooks for auth settings, identity and tokens ([9f684a3](https://github.com/metril/speedtest-tracker/commit/9f684a3c415e19dbaa6d9c3e1486f5877a23e04f))
* **web:** API client, React Query hooks and value formatters ([b081bfa](https://github.com/metril/speedtest-tracker/commit/b081bfa2c2e80625b447c33768015be6f8595118))
* **web:** API token management panel ([ff4eee1](https://github.com/metril/speedtest-tracker/commit/ff4eee1d084adddffebad95d4ab6a7a0ab90438b))
* **web:** auth settings section, env-lock badges and an open-mode warning banner ([30e3c65](https://github.com/metril/speedtest-tracker/commit/30e3c650a68a7a023e4629ee806ee8e477d0eb88))
* **web:** chart target history and the outage timeline on the dashboard ([7d2f459](https://github.com/metril/speedtest-tracker/commit/7d2f459f161e7f527cd1a312db63d5ee096e9a3f))
* **web:** embed SPA assets with fallback and cache headers ([ca95b65](https://github.com/metril/speedtest-tracker/commit/ca95b65a7346fda81a819db383064b128a387585))
* **web:** live run banner and virtualised results table with filters ([ed7eeed](https://github.com/metril/speedtest-tracker/commit/ed7eeed82d1664b4f4263cd11fb11351b3133da0))
* **web:** notifications settings section and channel editor ([d9e3791](https://github.com/metril/speedtest-tracker/commit/d9e3791fc0165c38276173176dabe9df742fc647))
* **web:** notifications settings types and channel test hook ([61b685c](https://github.com/metril/speedtest-tracker/commit/61b685ccac22c17a2ef9ba966099f8f949f35375))
* **web:** Ookla-style live run panel with gauge, sparkline and target stepper ([8fc3d70](https://github.com/metril/speedtest-tracker/commit/8fc3d7023c2b7142b3d2dd31e9ddf1a405cf0ba2))
* **web:** per-target notification thresholds in the target form ([9a5f705](https://github.com/metril/speedtest-tracker/commit/9a5f705029ce9402c19a12def2451b00e7e8be96))
* **web:** schedules API client and query hooks ([401b8ac](https://github.com/metril/speedtest-tracker/commit/401b8aca711d5cb68fa3aeff0756ad1a47f9da88))
* **web:** schedules page with cron helper, ordered targets and overlap warnings ([12476ea](https://github.com/metril/speedtest-tracker/commit/12476ea015b6bf6459724f903912f70abd4ba970))
* **web:** settings API client and query hooks ([2667232](https://github.com/metril/speedtest-tracker/commit/26672325bb0e6ed7d9b5e1513d0e9890d733094a))
* **web:** settings page with general, engines and integrations ([97cc3bd](https://github.com/metril/speedtest-tracker/commit/97cc3bdbceb26e0d516756b2bb713445184002cd))
* **web:** speed gauge and throughput sparkline components ([4e92263](https://github.com/metril/speedtest-tracker/commit/4e9226307b42eb245f3e7b4acfd1f2a4bdc13a0d))
* **web:** targets page with per-engine option forms and run-now ([6796c3b](https://github.com/metril/speedtest-tracker/commit/6796c3b8d49466221be71d371f99737a971117b1))


### Bug Fixes

* **api:** adapt runs listing to store.RunFilter ([cac429d](https://github.com/metril/speedtest-tracker/commit/cac429d2701f2bc4f9aab0d6d2d959cf3465da4a))
* **api:** bound the summary cache and restrict summary to named ranges ([16f549f](https://github.com/metril/speedtest-tracker/commit/16f549fa1eb7c8b3aeb0f080ee853a0c70bb05d5))
* **api:** default outages window to seven days ([4584d8d](https://github.com/metril/speedtest-tracker/commit/4584d8dc956b62a2e4d553d1cf25c85191b684b2))
* **api:** detach reload from request ctx, reject dup/zero ids ([88f547b](https://github.com/metril/speedtest-tracker/commit/88f547bf715eb11490483a9e243730350f40d78a))
* **api:** harden CSV export against formula injection and long exports ([248ed50](https://github.com/metril/speedtest-tracker/commit/248ed5022a222c98b6e256a1e4158e3efeecb9ca))
* **api:** map non-not-found errors to 500, count tags in runes, treat null options as default ([f64505f](https://github.com/metril/speedtest-tracker/commit/f64505f4496aa950c448517ef5f7cbdf8cc06548))
* **api:** mask and safely restore all channel secrets, not just the token ([ba79d93](https://github.com/metril/speedtest-tracker/commit/ba79d930a0ca94f430871bc038f629b13706774b))
* **api:** reexecute replays exact snapshot, add tag filter, from/to validation, tag list limits ([01778b8](https://github.com/metril/speedtest-tracker/commit/01778b85c66b8efc5b260c07583d40bdbe011994))
* **api:** require admin identity for token routes ([b12e072](https://github.com/metril/speedtest-tracker/commit/b12e0727b9efa86bc279c972136eeab9d39ebd2d))
* **api:** return no fire times for disabled schedules ([c187c69](https://github.com/metril/speedtest-tracker/commit/c187c69b8671bac61c122dfee6f03c93dc420316))
* **api:** stop leaking the stored integration secret to arbitrary hosts ([91e0625](https://github.com/metril/speedtest-tracker/commit/91e0625815679e579dceb896e2ebd8335ca8651a))
* **api:** tighten auth-section writes and the lockout guard ([48b5b13](https://github.com/metril/speedtest-tracker/commit/48b5b130935798f03b8d78622726bd720fd955fe))
* **api:** validate target thresholds on create/update ([c658343](https://github.com/metril/speedtest-tracker/commit/c658343bc96e15238d5359f7ad727a762d7ea5d6))
* **auth:** identify token source and dispatch touches off request path ([8675fcb](https://github.com/metril/speedtest-tracker/commit/8675fcb7b946d1d2161cf8fcc769fcc12b637dbe))
* **build:** drop redundant git checkout of dist/.gitkeep ([909b7bf](https://github.com/metril/speedtest-tracker/commit/909b7bf42c3f06a7790d58cb734c886ff35b3ee5))
* **cmd:** stop the auth touch worker on shutdown; clarify recovery docs ([bf3e64d](https://github.com/metril/speedtest-tracker/commit/bf3e64d2f8ec484ebe649c450b50a5053dfbce04))
* correct cloudflare download byte accounting and naming ([3094ca5](https://github.com/metril/speedtest-tracker/commit/3094ca5902829a3662fc1491211e187c5ded0123))
* harden iperf3 engine error handling and auth ([1a83e37](https://github.com/metril/speedtest-tracker/commit/1a83e37b6cdfa82a67be720351ae94c1f5052bd3))
* **notify:** drain queue on stop and stop mislabeling suppressed alerts ([40b0a6e](https://github.com/metril/speedtest-tracker/commit/40b0a6e28fac181dda51099dd9961c76c4ad096a))
* **ookla:** kill process group on cancel, keep parsed result, surface stderr ([4a8a1b5](https://github.com/metril/speedtest-tracker/commit/4a8a1b52bb3a723e345d30361199284255704290))
* propagate context cancellation from ookla.Run ([f3a67ab](https://github.com/metril/speedtest-tracker/commit/f3a67aba81edf06e8f4b0cd22374a9c63e80857a))
* prune immediately on start and cache metrics_enabled for scrapes ([ae2576b](https://github.com/metril/speedtest-tracker/commit/ae2576b718e9e19aff821cffe51405e6d6c2157d))
* read the shared settings DB in run_cmd instead of fabricating engine config ([22b0bff](https://github.com/metril/speedtest-tracker/commit/22b0bff493026f89634c7676f645d26f26eb04e0))
* remove dead fake.Engine.Result field, add settings.General default fallback ([5627a28](https://github.com/metril/speedtest-tracker/commit/5627a281dfb13eb74d6ee4d0d4efbb492ae1530c))
* **runner:** avoid holding r.mu across store I/O, fix shutdown/cancel races ([908357c](https://github.com/metril/speedtest-tracker/commit/908357c9a7761b0ef46ae477b159ac1c10b7b133))
* **runner:** close residual races in shutdown claim and cancel-vs-running ([a237928](https://github.com/metril/speedtest-tracker/commit/a2379282177366a22e61e793cfb4aeb58af3a3b0))
* **runner:** publish queued event before lane dispatch ([482c4c3](https://github.com/metril/speedtest-tracker/commit/482c4c3f249ca45ac1ecf979b9dbb50054da24c6))
* **runner:** serialize enqueue/shutdown, correct cancel and shutdown semantics ([39d4c8a](https://github.com/metril/speedtest-tracker/commit/39d4c8a4b8436575204d03674dcc99fee308a077))
* **scheduler:** serialize Reload against concurrent Reload/Stop ([8fd4f4a](https://github.com/metril/speedtest-tracker/commit/8fd4f4ae8fdbccfff3eae4b7f61bfe459d3d64c0))
* **settings:** always apply ST_AUTH_MODE and retry type mismatches as strings ([8890189](https://github.com/metril/speedtest-tracker/commit/8890189cebb272a2629834217aac0b0ee45ae0d5))
* **sse:** log marshal failures, flush via ResponseController ([3d4223c](https://github.com/metril/speedtest-tracker/commit/3d4223c9ae8d0e94c5167f7e8abac80907efb397))
* **store:** aggregate by status, not by nonzero value ([5ce6a1b](https://github.com/metril/speedtest-tracker/commit/5ce6a1b69ec867e3ce6b1cf90c9597b7976ee483))
* **store:** close outage incidents on recovery, sort and cap ([ced03a7](https://github.com/metril/speedtest-tracker/commit/ced03a7a78b274721c7f2883146dee2f429d329d))
* **store:** drop redundant history index migration ([08b858e](https://github.com/metril/speedtest-tracker/commit/08b858eb38b0c15f4948fa745e4bf69a86e7482b))
* **store:** only map sql.ErrNoRows to ErrNotFound in SetResultTags ([84597fb](https://github.com/metril/speedtest-tracker/commit/84597fbfb5a5bc1212c811cdb53b3a43f399d04f))
* **store:** scope _txlock=immediate to the write pool only ([a2991de](https://github.com/metril/speedtest-tracker/commit/a2991de7c38e5a380fce7c0eea785fb52cecb581))
* **vlpush:** give the final flush its own deadline instead of the cancelled reqCtx ([e26185f](https://github.com/metril/speedtest-tracker/commit/e26185fd3e79d5c376e4d666a7a3cf0ee88908b4))
* **vmpush:** drain the ring on shutdown and stop retrying once disabled ([89daf49](https://github.com/metril/speedtest-tracker/commit/89daf49fa621969f57d752736d0537119cf10674))
* **web:** a11y and lifecycle fixes for the live run dialog ([b0c469c](https://github.com/metril/speedtest-tracker/commit/b0c469cbc840a8db55efd9db7c850ce510ea365a))
* **web:** a11y roles for the results table and live run banner ([492fc40](https://github.com/metril/speedtest-tracker/commit/492fc40d306b57ced2868ca175f07df67fbff96b))
* **web:** arrow-key navigation for the dashboard range picker ([806281e](https://github.com/metril/speedtest-tracker/commit/806281e01e165f554567627e6cf9d52cecabae06))
* **web:** confirm target deletion, per-row run state, iperf3 checkbox lockout ([7756e85](https://github.com/metril/speedtest-tracker/commit/7756e8581f0e8af5675b96a933f8f775b803654c))
* **web:** convert result filter dates through local time, add tag filter ([c6f863f](https://github.com/metril/speedtest-tracker/commit/c6f863ff0820d99df66d335ceba530a1cb31692d))
* **web:** correct token API paths and trim trusted-proxy lines before save ([b6d23ea](https://github.com/metril/speedtest-tracker/commit/b6d23eaa993b6ff29589812ff83a8a72ee0c8921))
* **web:** guard live run state against cross-run and stale events ([49cfe5e](https://github.com/metril/speedtest-tracker/commit/49cfe5e129e0598e85851371c34707342adb82ea))
* **web:** iperf3 password field, mutual-exclusion hints, debounced ookla search ([77d3f49](https://github.com/metril/speedtest-tracker/commit/77d3f49420fc82fae6f2a2ebbe22567db6ba1550))
* **web:** keep live query caches and dashboard charts in step ([cf5c9f0](https://github.com/metril/speedtest-tracker/commit/cf5c9f096aade7f3052426b91d3a7367183ee019))
* **web:** live-refresh caches on run/result events, confirm before delete, banner grace period ([2fcfbe1](https://github.com/metril/speedtest-tracker/commit/2fcfbe1ae470584f04c4773a29f9b50ba3696b06))
* **web:** make the theme toggle keyboard-accessible and fix token contrast ([449abbb](https://github.com/metril/speedtest-tracker/commit/449abbb8614f656246a5b16b16fdebca13e055e2))
* **web:** preserve channel secrets on type change, scope test-channel state ([c2edf05](https://github.com/metril/speedtest-tracker/commit/c2edf054b7b96c0cd58941c5334a2b7857549a7a))
* **web:** preserve unsaved settings edits and stabilize label rows ([4125768](https://github.com/metril/speedtest-tracker/commit/41257685effba7c19a539902d7c04145d446feeb))
* **web:** remount schedule form per edit, warnings at page level ([0be2e63](https://github.com/metril/speedtest-tracker/commit/0be2e6322b63b610acfb6df10d01baf34e5cabea))
* **web:** restore internal/web/dist/.gitkeep via build plugin ([d486b82](https://github.com/metril/speedtest-tracker/commit/d486b82e375d52f9f1a5787c2af7da18b6a560fc))
* **web:** scope live region to run status text ([5fb6d0b](https://github.com/metril/speedtest-tracker/commit/5fb6d0b77223be199b09b5b514a7874533ab1331))
* **web:** treat directories as not-found in the SPA handler ([e753f2e](https://github.com/metril/speedtest-tracker/commit/e753f2eb7ff8bbe4e712de72200dd2d4907100b0))


### Performance Improvements

* **api:** cache the summary endpoint and add ETag revalidation to list endpoints ([d84fc1a](https://github.com/metril/speedtest-tracker/commit/d84fc1a52862f79cefaffd1a2b1dc1a6b4fe3303))
* **store:** index results by (target_id|status|engine, id DESC) ([f3c6826](https://github.com/metril/speedtest-tracker/commit/f3c68265c4382dad3ea02e60affb1fd7b52ecb1e))
* **web:** split routes into lazy chunks and drop dead client helpers ([868a15a](https://github.com/metril/speedtest-tracker/commit/868a15af6f48a3fcc5effe184fd547c119604e1c))

## Changelog

All notable changes to this project are documented in this file. The format is
maintained automatically by release-please from Conventional Commit messages.
