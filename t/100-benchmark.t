use Test::Nginx::Socket::Lua 'no_plan';

master_on();
workers(1);
log_level('error');

run_tests();

__DATA__

=== TEST 1: baseline 204 without the access hook
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
    location = /baseline {
        content_by_lua_block {
            return ngx.exit(204)
        }
    }
--- request
GET /baseline
--- error_code: 204
--- no_error_log
[error]


=== TEST 2: protected 204 with an exact IPv4 miss
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
