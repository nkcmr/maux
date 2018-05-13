describe({
    summary  = "Display directories as trees (with optional color/HTML output)",
    homepage = "http://mama.indstate.edu/users/ice/tree/",
    url      = "http://mama.indstate.edu/users/ice/tree/src/tree-1.7.0.tgz",
    mirror   = "https://fossies.org/linux/misc/tree-1.7.0.tgz",
    sha256   = "6957c20e82561ac4231638996e74f4cfa4e6faabc5a2f511f0b4e3940e8f7b12"
})

-- homebrew formula feature parity checklist
-- * option ------------| to have a "install --custom-cli-flag"
-- * deprecated_option -| same as above but make it deprecated
-- 

function install()
    env_set("CFLAGS", "-fomit-frame-pointer")
    local objs = "tree.o unix.o html.o xml.o hash.o color.o strverscmp.o json.o"
    exec(
        "make",
            "prefix=" .. prefix,
            "MANDIR=" .. man1,
            "CC=" .. env_get("CC"),
            "CFLAGS=" .. env_get("CFLAGS"),
            "LDFLAGS=" .. env_get("LDFLAGS"),
            "OBJS=" .. objs,
            "install"
    )
end

function test()
    assert_code("mything", "--version", { code = 1 })
end
