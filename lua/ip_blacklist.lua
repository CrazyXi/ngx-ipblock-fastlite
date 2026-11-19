local ffi = require "ffi"
local bit = require "bit"
local sha256 = require "resty.sha256"

local _M = { _VERSION = "1.4.0" }
local MAGIC = "IPBLV001"
local FORMAT_VERSION = 1
local HEADER_SIZE = 64
local DEFAULT_MAX_IPS = 2000000
local READ_WORDS = 16384 -- 64 KiB; divisible by four.
local PREFIX_COUNT = 65536 -- Top 16 bits: 256 KiB + 4 bytes; +240 KiB versus /12.
local PREFIX_DIVISOR = 65536

-- IMPORTANT: init() MUST run in init_by_lua*, before workers fork. Calling it
-- from init_worker_by_lua* would allocate/verify one independent array per worker
-- and defeat copy-on-write sharing. init() rejects the wrong ngx_lua phase.
-- Keep these references at module scope so the read-only array survives GC.
local suffixes
local prefix_offsets
local metadata
local sites = {}
local default_site = { enabled = false }
local initialized = false
local log_window_second = 0
local log_window_count = 0
local anomaly_window_second = 0
local anomaly_window_count = 0

local function u32le(s, offset)
    local a, b, c, d = string.byte(s, offset, offset + 3)
    return a + b * 256 + c * 65536 + d * 16777216
end

local function u64le(s, offset)
    local lo = u32le(s, offset)
    local hi = u32le(s, offset + 4)
    return lo + hi * 4294967296
end

local function hex(s)
    return (s:gsub(".", function(c)
        return string.format("%02x", string.byte(c))
    end))
end

local function parse_ipv4(raw)
    if type(raw) ~= "string" then
        return nil
    end
    local length = #raw
    if length < 7 or length > 15 then
        return nil
    end
    local bytes = ffi.cast("const uint8_t *", raw)
    local value = 0
    local octet = 0
    local digits = 0
    local dots = 0
    for i = 0, length - 1 do
        local byte = bytes[i]
        if byte >= 48 and byte <= 57 then
            if digits > 0 and octet == 0 then
                return nil -- non-canonical leading zero
            end
            octet = octet * 10 + byte - 48
            digits = digits + 1
            if octet > 255 or digits > 3 then
                return nil
            end
        elseif byte == 46 then
            if digits == 0 or dots >= 3 then
                return nil
            end
            value = value * 256 + octet
            octet = 0
            digits = 0
            dots = dots + 1
        else
            return nil
        end
    end
    if dots ~= 3 or digits == 0 then
        return nil
    end
    return value * 256 + octet
end

local function compile_whitelist(raw)
    local result = {}
    for _, ip in ipairs(raw or {}) do
        local value = parse_ipv4(ip)
        if value == nil then
            error("invalid whitelist IPv4: " .. tostring(ip))
        end
        result[value] = true
    end
    return result
end

local function normalize_site(raw)
    raw = raw or {}
    local sample_rate = raw.log_sample_rate or 0
    if sample_rate < 0 or sample_rate > 1 then
        error("log_sample_rate must be between 0 and 1")
    end
    local log_rate = raw.log_rate_per_second or 10
    if log_rate < 0 or log_rate ~= math.floor(log_rate) then
        error("log_rate_per_second must be a non-negative integer")
    end
    local anomaly_rate = raw.anomaly_log_rate_per_second or 2
    if anomaly_rate < 0 or anomaly_rate ~= math.floor(anomaly_rate) then
        error("anomaly_log_rate_per_second must be a non-negative integer")
    end
    local block_status = raw.block_status or 444
    local invalid_status = raw.invalid_status or ngx.HTTP_FORBIDDEN
    if block_status ~= 444 and (block_status < 400 or block_status > 599) then
        error("block_status must be 444 or an HTTP error status")
    end
    if invalid_status == 444 or invalid_status < 400 or invalid_status > 599 then
        error("invalid_status must be an HTTP error status other than 444")
    end
    return {
        enabled = raw.enabled == true,
        whitelist = compile_whitelist(raw.whitelist),
        -- Cloudflare validation is opt-in. Direct sites use $remote_addr.
        require_cf_header = raw.require_cf_header == true,
        fail_closed = raw.fail_closed ~= false,
        log_hits = raw.log_hits == true,
        log_sample_rate = sample_rate,
        log_rate_per_second = log_rate,
        anomaly_log_rate_per_second = anomaly_rate,
        block_status = block_status,
        invalid_status = invalid_status,
        forbidden_body = raw.forbidden_body or "Forbidden\n",
    }
end

local function load_binary(path, max_ips)
    local file, open_err = io.open(path, "rb")
    if not file then
        return nil, "open failed: " .. tostring(open_err)
    end
    local function fail(message)
        file:close()
        return nil, message
    end

    local header = file:read(HEADER_SIZE)
    if not header or #header ~= HEADER_SIZE then
        return fail("short header")
    end
    if header:sub(1, 8) ~= MAGIC then
        return fail("invalid magic")
    end
    if u32le(header, 9) ~= FORMAT_VERSION then
        return fail("unsupported format version")
    end
    if u32le(header, 13) ~= HEADER_SIZE then
        return fail("invalid header length")
    end
    local count = u64le(header, 17)
    if count < 1 or count > max_ips or count ~= math.floor(count) then
        return fail("invalid IP count: " .. tostring(count))
    end
    local end_position, seek_err = file:seek("end")
    if not end_position then
        return fail("seek failed: " .. tostring(seek_err))
    end
    local expected_size = HEADER_SIZE + count * 4
    if end_position ~= expected_size then
        return fail("file length " .. tostring(end_position) .. ", expected " .. tostring(expected_size))
    end
    file:seek("set", HEADER_SIZE)

    local hasher = sha256:new()
    if not hasher then
        return fail("cannot initialize SHA-256")
    end
    local array = ffi.new("uint16_t[?]", count)
    local offsets = ffi.new("uint32_t[?]", PREFIX_COUNT + 1)
    local next_prefix = 0
    local native_le = ffi.abi("le")
    local buffer = native_le and ffi.new("uint32_t[?]", math.min(count, READ_WORDS))
    local index = 0
    local previous
    while index < count do
        local words = math.min(READ_WORDS, count - index)
        local chunk = file:read(words * 4)
        if not chunk or #chunk ~= words * 4 then
            return fail("short address data")
        end
        if not hasher:update(chunk) then
            return fail("SHA-256 update failed")
        end
        if native_le then
            -- Copy bytes into the aligned FFI array before reading uint32s.
            -- Casting a Lua string directly to uint32_t* may be unaligned.
            ffi.copy(buffer, chunk, words * 4)
        end
        for word = 0, words - 1 do
            local value
            if native_le then
                value = tonumber(buffer[word])
            else
                value = u32le(chunk, word * 4 + 1)
            end
            if previous ~= nil and value <= previous then
                return fail("addresses are not strictly increasing at index " .. tostring(index))
            end
            local prefix = math.floor(value / PREFIX_DIVISOR)
            while next_prefix <= prefix do
                offsets[next_prefix] = index
                next_prefix = next_prefix + 1
            end
            array[index] = bit.band(value, 65535)
            previous = value
            index = index + 1
        end
        chunk = nil
        collectgarbage("step", 128)
    end
    file:close()
    local actual_digest = hasher:final()
    if not actual_digest or actual_digest ~= header:sub(33, 64) then
        return nil, "address data SHA-256 mismatch"
    end
    for prefix = next_prefix, PREFIX_COUNT do
        offsets[prefix] = count
    end
    local built_at = u64le(header, 25)
    return array, {
        format = MAGIC,
        version = FORMAT_VERSION,
        count = count,
        built_at_unix = built_at,
        data_sha256 = hex(actual_digest),
        path = path,
    }, offsets
end

local lookup_value

-- Compile once in the master. Only install a combined matcher when rules exist.
local function compile_cidrs(rules, exact_lookup)
    if rules == nil then return exact_lookup, 0 end
    if type(rules) ~= "table" then error("cidrs must be an array") end
    local ranges = {}
    local count = 0
    for i, raw in pairs(rules) do
        if type(i) ~= "number" or i < 1 or i ~= math.floor(i) then
            error("cidrs must be an array")
        end
        local ip, bits
        if type(raw) == "string" then ip, bits = raw:match("^([^/]+)/(%d%d?)$") end
        local first = parse_ipv4(ip)
        local prefix = tonumber(bits)
        if not first or not prefix or prefix > 32 or tostring(prefix) ~= bits then
            error("invalid IPv4 CIDR at index " .. i .. ": " .. tostring(raw))
        end
        local size = 2 ^ (32 - prefix)
        if first % size ~= 0 then error("CIDR has host bits set: " .. raw) end
        ranges[i] = {first, first + size - 1}
        count = count + 1
    end
    if count == 0 then return exact_lookup, 0 end
    for i = 1, count do
        if not ranges[i] then error("cidrs must be a dense array") end
    end
    table.sort(ranges, function(a, b) return a[1] < b[1] end)
    local merged = 0
    for i = 1, count do
        local current = ranges[i]
        local last = ranges[merged]
        if last and current[1] <= last[2] + 1 then
            last[2] = math.max(last[2], current[2])
        else
            merged = merged + 1
            ranges[merged] = current
        end
    end
    local bounds = ffi.new("uint32_t[?]", merged * 2)
    for i = 1, merged do
        bounds[i * 2 - 2] = ranges[i][1]
        bounds[i * 2 - 1] = ranges[i][2]
    end
    return function(target)
        if exact_lookup(target) then return true end
        local low, high = 0, merged - 1
        while low <= high do
            local mid = math.floor((low + high) / 2)
            if target < bounds[mid * 2] then
                high = mid - 1
            elseif target > bounds[mid * 2 + 1] then
                low = mid + 1
            else
                return true
            end
        end
        return false
    end, merged
end

function _M.init(options)
    if not ngx.get_phase or ngx.get_phase() ~= "init" then
        error("ip_blacklist.init must run in init_by_lua*, before workers fork")
    end
    if initialized then
        error("ip_blacklist.init must be called only once per Lua VM")
    end
    options = options or {}
    if type(options.path) ~= "string" or options.path == "" then
        error("ip_blacklist path is required")
    end
    local array, result, offsets = load_binary(options.path, options.max_ips or DEFAULT_MAX_IPS)
    if not array then
        error("ip_blacklist load failed: " .. tostring(result))
    end
    local compiled_sites = {}
    for host, config in pairs(options.sites or {}) do
        compiled_sites[string.lower(host)] = normalize_site(config)
    end
    suffixes = array
    prefix_offsets = offsets
    metadata = result
    sites = compiled_sites
    default_site = normalize_site(options.default_site)
    lookup_value, metadata.cidr_ranges = compile_cidrs(options.cidrs, lookup_value)
    compile_cidrs = nil
    initialized = true
    ngx.log(ngx.NOTICE, "ip_blacklist loaded version=", result.version,
        " count=", result.count, " built_at_unix=", result.built_at_unix,
        " data_sha256=", result.data_sha256)
    -- These functions and the local SHA module reference are init-only. Release
    -- them before workers settle so they do not inflate the steady Lua heap.
    sha256 = nil
    load_binary = nil
    normalize_site = nil
    compile_whitelist = nil
    collectgarbage("collect")
    return true
end

lookup_value = function(target)
    -- Keep prefix division; midpoint shifts were benchmarked independently.
    local prefix = math.floor(target / PREFIX_DIVISOR)
    local low = tonumber(prefix_offsets[prefix])
    local high = tonumber(prefix_offsets[prefix + 1]) - 1
    local suffix = bit.band(target, 65535)
    while low <= high do
        -- A /16 bucket has at most 65536 distinct addresses. Shift the bounded
        -- distance, not low + high, so large absolute offsets cannot wrap.
        local middle = low + bit.rshift(high - low, 1)
        local current = suffixes[middle]
        if current < suffix then
            low = middle + 1
        elseif current > suffix then
            high = middle - 1
        else
            return true
        end
    end
    return false
end

function _M.lookup(raw_ip)
    if not initialized then
        return nil, "not initialized"
    end
    local target = parse_ipv4(raw_ip)
    if target == nil then
        return nil, "not a canonical IPv4 address"
    end
    return lookup_value(target)
end

local function should_log(config, ip, host)
    if not config.log_hits or config.log_sample_rate <= 0 or config.log_rate_per_second == 0 then
        return false
    end
    -- Once this worker's quota is exhausted, avoid hashing every attack request.
    -- Sampling failures must not consume quota; increment only after sampling.
    local now = ngx.time()
    if log_window_second ~= now then
        log_window_second = now
        log_window_count = 0
    end
    if log_window_count >= config.log_rate_per_second then
        return false
    end
    if config.log_sample_rate < 1 then
        -- No per-hit concatenation or tostring allocation under attack load.
        -- Rotate the host hash so that each component influences different bits.
        local sample = bit.band(bit.bxor(ngx.crc32_short(ip),
            bit.rol(ngx.crc32_short(host), 13), now), 0x7fffffff)
        if (sample % 1000000) >= config.log_sample_rate * 1000000 then
            return false
        end
    end
    log_window_count = log_window_count + 1
    return true
end

local function should_log_anomaly(config)
    local now = ngx.time()
    if anomaly_window_second ~= now then
        anomaly_window_second = now
        anomaly_window_count = 0
    end
    if anomaly_window_count >= config.anomaly_log_rate_per_second then
        return false
    end
    anomaly_window_count = anomaly_window_count + 1
    return true
end

local function reject(reason, config, status)
    if status == 444 then
        -- Nginx 444 intentionally closes the connection without an HTTP response.
        return ngx.exit(444)
    end
    ngx.header["Cache-Control"] = "no-store"
    ngx.status = status
    ngx.header["Content-Type"] = "text/plain; charset=utf-8"
    ngx.print(config.forbidden_body)
    return ngx.exit(status)
end

function _M.access()
    if not initialized then
        error("ip_blacklist is not initialized")
    end
    -- Nginx's $host is already normalized to lower case.
    local host = ngx.var.host or ""
    local config = sites[host] or default_site
    if not config.enabled then
        return
    end

    local visitor = ngx.var.remote_addr
    if config.require_cf_header then
        local cf_ip = ngx.var.http_cf_connecting_ip
        local restored = cf_ip and cf_ip ~= "" and cf_ip == visitor
        if not restored and cf_ip and visitor and cf_ip:find(":", 1, true) and visitor:find(":", 1, true) then
            restored = string.lower(cf_ip) == string.lower(visitor)
        end
        if not restored then
            if should_log_anomaly(config) then
                ngx.log(ngx.WARN, "ip_blacklist rejected invalid CF visitor address host=", host)
            end
            if config.fail_closed then
                return reject("cf_ip_restore_failed", config, config.invalid_status)
            end
            return
        end
    end
    if type(visitor) ~= "string" then
        if config.fail_closed then
            return reject("missing_visitor_ip", config, config.invalid_status)
        end
        return
    end
    if visitor:find(":", 1, true) then
        return -- IPv6 is deliberately outside v1.
    end
    local value = parse_ipv4(visitor)
    if value == nil then
        if should_log_anomaly(config) then
            ngx.log(ngx.WARN, "ip_blacklist rejected malformed visitor IPv4 host=", host)
        end
        if config.fail_closed then
            return reject("invalid_visitor_ip", config, config.invalid_status)
        end
        return
    end
    if config.whitelist[value] then
        return
    end
    local blocked = lookup_value(value)
    if not blocked then
        return
    end
    if should_log(config, visitor, host) then
        ngx.log(ngx.WARN, "ip_blacklist hit ip=", visitor, " host=", host,
            " data_sha256=", metadata.data_sha256)
    end
    return reject("blacklisted", config, config.block_status)
end

function _M.status()
    if not initialized then
        return { initialized = false }
    end
    return {
        initialized = true,
        module_version = _M._VERSION,
        cidr_ranges = metadata.cidr_ranges,
        format = metadata.format,
        format_version = metadata.version,
        count = metadata.count,
        built_at_unix = metadata.built_at_unix,
        data_sha256 = metadata.data_sha256,
        path = metadata.path,
    }
end

return _M
