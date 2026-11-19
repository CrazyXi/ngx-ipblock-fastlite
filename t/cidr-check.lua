-- Run inside init_by_lua* with the module file and bundled snapshot paths.
return function(module_path, snapshot)
    local function load_rules(rules)
        local m = dofile(module_path)
        m.init({path = snapshot, cidrs = rules})
        return m
    end
    local m = load_rules({"255.255.255.255/32", "192.0.2.128/25", "0.0.0.0/32",
                          "192.0.2.0/25", "192.0.2.0/26", "192.0.2.0/25"})
    assert(m.status().cidr_ranges == 3)
    for _, ip in ipairs({"192.0.2.0", "192.0.2.127", "192.0.2.128", "192.0.2.255",
                         "255.255.255.255", "0.0.0.0", "1.0.164.165"}) do
        assert(m.lookup(ip) == true, ip)
    end
    for _, ip in ipairs({"192.0.1.255", "192.0.3.0", "255.255.255.254"}) do
        assert(m.lookup(ip) == false, ip)
    end
    local all = load_rules({"0.0.0.0/0"})
    assert(all.lookup("0.0.0.0") and all.lookup("255.255.255.255"))
    assert(all.lookup("::1") == nil)
    local off = load_rules({})
    assert(off.status().cidr_ranges == 0 and off.lookup("192.0.2.0") == false)
    for _, invalid in ipairs({{"192.0.2.1/24"}, {"::/0"}, {"0.0.0.0/33"},
                               {"0.0.0.0/00"}, {false}, {[2]="192.0.2.0/24"}}) do
        local ok, err = pcall(load_rules, invalid)
        assert(not ok and tostring(err):lower():find("cidr"), tostring(err))
    end
end
