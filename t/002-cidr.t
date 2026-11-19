use Test::Nginx::Socket::Lua 'no_plan';

master_on();
workers(1);
run_tests();

__DATA__

=== TEST 1: CIDR bounds, overlap, configuration validation and allowlist precedence
--- http_config
    lua_package_path "$TEST_NGINX_IPBLOCK_LUA/?.lua;;";
    init_by_lua_block {
        dofile("$TEST_NGINX_IPBLOCK_LUA/../t/cidr-check.lua")(
            "$TEST_NGINX_IPBLOCK_LUA/ip_blacklist.lua", "$TEST_NGINX_IPBLOCK_BIN")
        require("ip_blacklist").init({
            path = "$TEST_NGINX_IPBLOCK_BIN",
            cidrs = {"127.0.0.0/8"},
            default_site = {enabled = true, whitelist = {"127.0.0.1"}},
        })
    }
--- config
    location = /test {
        access_by_lua_block { require("ip_blacklist").access() }
        content_by_lua_block { ngx.say("allowed") }
    }
--- request
GET /test
--- response_body
allowed
