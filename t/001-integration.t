use Test::Nginx::Socket::Lua 'no_plan';

no_shuffle();
master_on();
workers(1);
log_level('warn');

run_tests();

__DATA__

=== TEST 1: exact lookup finds an address from the fixture
--- http_config
    lua_package_path "$TEST_NGINX_IPBLOCK_LUA/?.lua;;";
    init_by_lua_block {
        require("ip_blacklist").init({
            path = "$TEST_NGINX_IPBLOCK_BIN",
            default_site = { enabled = false },
        })
    }
--- config
    location = /lookup {
        content_by_lua_block {
            local hit, err = require("ip_blacklist").lookup(ngx.var.arg_ip)
            if hit == nil then
                ngx.status = 400
                ngx.say(err)
                return
            end
            ngx.say(hit and "hit" or "miss")
        }
    }
--- request
GET /lookup?ip=1.0.164.165
--- response_body
hit
--- no_error_log
[error]


=== TEST 2: exact lookup misses an address outside the fixture
--- http_config
    lua_package_path "$TEST_NGINX_IPBLOCK_LUA/?.lua;;";
    init_by_lua_block {
        require("ip_blacklist").init({
            path = "$TEST_NGINX_IPBLOCK_BIN",
            default_site = { enabled = false },
        })
    }
--- config
    location = /lookup {
        content_by_lua_block {
            local hit, err = require("ip_blacklist").lookup(ngx.var.arg_ip)
            if hit == nil then
                ngx.status = 400
                ngx.say(err)
                return
            end
            ngx.say(hit and "hit" or "miss")
        }
    }
--- request
GET /lookup?ip=192.0.2.1
--- response_body
miss
--- no_error_log
[error]


=== TEST 3: access phase allows a non-listed client
--- http_config
    lua_package_path "$TEST_NGINX_IPBLOCK_LUA/?.lua;;";
    init_by_lua_block {
        require("ip_blacklist").init({
            path = "$TEST_NGINX_IPBLOCK_BIN",
            default_site = {
                enabled = true,
                log_hits = false,
            },
        })
    }
--- config
    location = /protected {
        access_by_lua_block {
            require("ip_blacklist").access()
        }
        content_by_lua_block {
            return ngx.exit(204)
        }
    }
--- request
GET /protected
--- error_code: 204
--- no_error_log
[error]


=== TEST 4: access phase rejects a listed client before content
--- http_config
    lua_package_path "$TEST_NGINX_IPBLOCK_LUA/?.lua;;";
    init_by_lua_block {
        require("ip_blacklist").init({
            path = "$TEST_NGINX_IPBLOCK_BIN",
            default_site = {
                enabled = true,
                block_status = 403,
                log_hits = false,
            },
        })
    }
--- config
    set_real_ip_from 127.0.0.1;
    real_ip_header X-Test-Client-IP;

    location = /protected {
        access_by_lua_block {
            require("ip_blacklist").access()
        }
        content_by_lua_block {
            ngx.say("content-ran")
        }
    }
--- request
GET /protected
--- more_headers
X-Test-Client-IP: 1.0.164.165
--- error_code: 403
--- response_body
Forbidden
--- no_error_log
[error]


=== TEST 5: exact suffix lookup across a loader chunk and empty buckets
--- http_config
    lua_package_path "$TEST_NGINX_IPBLOCK_LUA/?.lua;;";
    init_by_lua_block {
        require("ip_blacklist").init({path = "$TEST_NGINX_IPBLOCK_BIN"})
    }
--- config
    location = /lookup {
        content_by_lua_block {
            local lookup = require("ip_blacklist").lookup
            -- Consecutive snapshot members on opposite sides of the 64 KiB chunk boundary.
            assert(lookup("34.51.168.107") and lookup("34.51.169.76"))
            assert(lookup("34.51.168.108") == false and lookup("34.51.169.77") == false)
            assert(lookup("16.5.0.0") and lookup("1.0.164.165"))
            assert(lookup("127.0.164.165") == false and lookup("255.255.255.255") == false)
            ngx.say("ok")
        }
    }
--- request
GET /lookup
--- response_body
ok
--- no_error_log
[error]
