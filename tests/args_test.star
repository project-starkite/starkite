# args_test.star - Comprehensive tests for the args module

# ===========================================================================
# Module exports and functions
# ===========================================================================

def test_args_module_exports():
    """Verify args module exports all expected builders and parse function."""
    exported = dir(args)
    assert("string" in exported, "args.string should be exported")
    assert("int" in exported, "args.int should be exported")
    assert("bool" in exported, "args.bool should be exported")
    assert("float" in exported, "args.float should be exported")
    assert("list" in exported, "args.list should be exported")
    assert("positional" in exported, "args.positional should be exported")
    assert("parse" in exported, "args.parse should be exported")

    # Verify TryModule wraps builders with try_ variants
    assert("try_string" in exported, "args.try_string should be exported")
    assert("try_int" in exported, "args.try_int should be exported")
    assert("try_bool" in exported, "args.try_bool should be exported")
    assert("try_float" in exported, "args.try_float should be exported")
    assert("try_list" in exported, "args.try_list should be exported")
    assert("try_positional" in exported, "args.try_positional should be exported")
    assert("try_parse" in exported, "args.try_parse should be exported")

# ===========================================================================
# String flag tests
# ===========================================================================

def test_args_string_valid():
    """Verify valid args.string declarations."""
    res = args.try_string(
        "cluster-action",
        flag = "action",
        shorthand = "a",
        default = "install",
        required = True,
        choices = ["install", "upgrade", "rollback"],
        help = "Action to execute on cluster",
        var_fallback = "ACTION",
    )
    assert(res.ok, "args.try_string with all valid parameters should succeed")

def test_args_string_empty_name():
    """Verify empty string flag name produces an error."""
    res = args.try_string("")
    assert(not res.ok, "empty name must fail")
    assert("name cannot be empty" in res.error, "error message must indicate empty name")

def test_args_string_invalid_default_type():
    """Verify non-string default value fails."""
    res = args.try_string("action", default = 12345)
    assert(not res.ok, "non-string default must fail")
    assert("must be a string" in res.error, "error message must indicate invalid default type")

def test_args_string_default_not_in_choices():
    """Verify default value not in choices list fails."""
    res = args.try_string(
        "action",
        default = "destroy",
        choices = ["setup-keys", "install"],
    )
    assert(not res.ok, "default not in choices must fail")
    assert("not in allowed choices" in res.error, "error message must indicate choice violation")

def test_args_string_invalid_choices_item():
    """Verify choices containing non-string items fails."""
    res = args.try_string(
        "action",
        choices = ["valid", 123],
    )
    assert(not res.ok, "non-string choice item must fail")
    assert("choices items must be strings" in res.error, "error message must indicate item type error")

def test_args_string_invalid_shorthand():
    """Verify multi-character shorthand produces an error."""
    res = args.try_string("action", shorthand = "act")
    assert(not res.ok, "multi-character shorthand must fail")
    assert("must be a single character" in res.error, "error message must indicate single character constraint")

# ===========================================================================
# Int flag tests
# ===========================================================================

def test_args_int_valid():
    """Verify valid args.int declarations with bounds."""
    res = args.try_int(
        "replicas",
        shorthand = "r",
        default = 3,
        min = 1,
        max = 10,
        required = True,
        help = "Replica count",
        var_fallback = "REPLICAS",
    )
    assert(res.ok, "valid args.int with bounds should succeed")

def test_args_int_bounds_violation():
    """Verify min greater than max fails."""
    res = args.try_int("replicas", min = 10, max = 2)
    assert(not res.ok, "min > max must fail")
    assert("cannot be greater than max" in res.error, "error message must indicate min > max")

def test_args_int_default_less_than_min():
    """Verify default value below min fails."""
    res = args.try_int("replicas", default = 0, min = 1)
    assert(not res.ok, "default < min must fail")
    assert("cannot be less than min" in res.error, "error message must indicate default < min")

def test_args_int_default_greater_than_max():
    """Verify default value above max fails."""
    res = args.try_int("replicas", default = 20, max = 10)
    assert(not res.ok, "default > max must fail")
    assert("cannot be greater than max" in res.error, "error message must indicate default > max")

def test_args_int_invalid_default_type():
    """Verify non-int default fails."""
    res = args.try_int("replicas", default = "three")
    assert(not res.ok, "non-int default must fail")
    assert("must be an int" in res.error, "error message must indicate type error")

# ===========================================================================
# Bool flag tests
# ===========================================================================

def test_args_bool_valid():
    """Verify valid args.bool declarations."""
    res = args.try_bool(
        "dry-run",
        shorthand = "d",
        default = False,
        help = "Simulate operations without changes",
    )
    assert(res.ok, "valid args.bool should succeed")

def test_args_bool_invalid_default():
    """Verify non-bool default fails."""
    res = args.try_bool("verbose", default = "true")
    assert(not res.ok, "non-bool default must fail")
    assert("must be a bool" in res.error, "error message must indicate type error")

# ===========================================================================
# Float flag tests
# ===========================================================================

def test_args_float_valid():
    """Verify valid args.float declarations."""
    res = args.try_float(
        "sampling-ratio",
        shorthand = "s",
        default = 0.75,
        min = 0.0,
        max = 1.0,
        help = "Sampling ratio between 0.0 and 1.0",
    )
    assert(res.ok, "valid args.float should succeed")

def test_args_float_accepts_int_bounds():
    """Verify float builder accepts integer min/max/default and converts."""
    res = args.try_float("rate", default = 1, min = 0, max = 5)
    assert(res.ok, "int values for float builder should succeed")

def test_args_float_bounds_violation():
    """Verify float min > max fails."""
    res = args.try_float("rate", min = 2.5, max = 1.0)
    assert(not res.ok, "min > max must fail")
    assert("cannot be greater than max" in res.error, "error message must indicate bounds error")

def test_args_float_default_out_of_bounds():
    """Verify float default out of range fails."""
    res = args.try_float("rate", default = 5.5, max = 2.0)
    assert(not res.ok, "default > max must fail")
    assert("cannot be greater than max" in res.error, "error message must indicate out of bounds")

# ===========================================================================
# List flag tests
# ===========================================================================

def test_args_list_string():
    """Verify list of strings declaration."""
    res = args.try_list(
        "workers",
        shorthand = "w",
        default = ["worker-1", "worker-2"],
        item_type = "string",
        help = "Worker node hostnames",
    )
    assert(res.ok, "valid string list should succeed")

def test_args_list_int():
    """Verify list of integers declaration."""
    res = args.try_list(
        "ports",
        default = [80, 443, 8080],
        item_type = "int",
        help = "Listening ports",
    )
    assert(res.ok, "valid int list should succeed")

def test_args_list_float():
    """Verify list of floats declaration."""
    res = args.try_list(
        "weights",
        default = [0.25, 0.75],
        item_type = "float",
        help = "Traffic weights",
    )
    assert(res.ok, "valid float list should succeed")

def test_args_list_invalid_item_type():
    """Verify unsupported item_type fails."""
    res = args.try_list("bad", item_type = "dict")
    assert(not res.ok, "unsupported item_type must fail")
    assert("invalid item_type" in res.error, "error message must indicate invalid item_type")

def test_args_list_default_item_mismatch():
    """Verify default elements not matching item_type fails."""
    res = args.try_list("ports", default = [80, "invalid-port"], item_type = "int")
    assert(not res.ok, "mismatched default item must fail")
    assert("default list element must be int" in res.error, "error message must indicate type mismatch")

# ===========================================================================
# Positional argument tests
# ===========================================================================

def test_args_positional_valid():
    """Verify valid positional arguments in correct order."""
    res1 = args.try_positional("cluster-name", required = True, help = "Cluster identifier")
    assert(res1.ok, "required positional should succeed")

    res2 = args.try_positional("config-file", default = "config.yaml", help = "Config path")
    assert(res2.ok, "optional positional following required should succeed")

def test_args_positional_empty_name():
    """Verify empty positional name fails."""
    res = args.try_positional("")
    assert(not res.ok, "empty positional name must fail")
    assert("name cannot be empty" in res.error, "error message must indicate empty name")

def test_args_positional_required_with_default():
    """Verify positional marked required=True with non-None default fails."""
    res = args.try_positional("target", required = True, default = "local")
    assert(not res.ok, "required with default must fail")
    assert("cannot be both required and have a default value" in res.error, "error message must indicate conflict")

# ===========================================================================
# Duplicate and collision tests
# ===========================================================================

def test_args_duplicate_flag_collision():
    """Verify declaring two flags with same CLI flag name fails."""
    res1 = args.try_string("action-one", flag = "action")
    assert(res1.ok, "first flag declaration should succeed")

    res2 = args.try_string("action-two", flag = "action")
    assert(not res2.ok, "second flag with same CLI flag name must fail")
    assert("already defined" in res2.error, "error message must indicate collision")

def test_args_duplicate_shorthand_collision():
    """Verify declaring two flags with same shorthand fails."""
    res1 = args.try_string("force", shorthand = "f")
    assert(res1.ok, "first shorthand declaration should succeed")

    res2 = args.try_bool("fast", shorthand = "f")
    assert(not res2.ok, "second flag with duplicate shorthand must fail")
    assert("shorthand -f is already defined" in res2.error, "error message must indicate shorthand collision")

def test_args_positional_and_flag_attribute_collision():
    """Verify positional argument conflicting with flag attribute fails."""
    res1 = args.try_string("node-name")
    assert(res1.ok, "flag declaration should succeed")

    res2 = args.try_positional("node_name")
    assert(not res2.ok, "positional with conflicting attribute must fail")
    assert("conflicts with an existing argument" in res2.error, "error message must indicate attribute conflict")

# ===========================================================================
# Schema declaration & parse stub execution
# ===========================================================================

def test_args_full_schema_and_parse():
    """Verify complete schema declaration and execution of args.parse()."""
    args.string("environment", shorthand = "e", default = "staging", choices = ["dev", "staging", "prod"])
    args.int("concurrency", shorthand = "c", default = 4, min = 1, max = 32)
    args.bool("dry-run", shorthand = "d")
    args.float("threshold", default = 0.8, min = 0.0, max = 1.0)
    args.list("tags", default = ["k8s", "app"])
    args.positional("cluster-target", default = "default-cluster")

    p = args.parse()
    assert(p != None, "args.parse() should return non-None struct")
    assert(p.environment == "staging", "default environment")
    assert(p.concurrency == 4, "default concurrency")
    assert(p.dry_run == False, "default dry_run")
    assert(p.threshold == 0.8, "default threshold")
    assert(len(p.tags) == 2, "default tags count")
    assert(p.tags[0] == "k8s", "tag 0")
    assert(p.cluster_target == "default-cluster", "default cluster-target")

    # Verify .get() method
    assert(p.get("environment") == "staging", "p.get('environment')")
    assert(p.get("missing", "fallback") == "fallback", "p.get with fallback")
    assert(p.get("missing") == None, "p.get without fallback")

    # Verify dictionary indexing
    assert(p["environment"] == "staging", "p['environment']")
    assert(p["cluster_target"] == "default-cluster", "p['cluster_target']")

def test_args_try_parse_missing_required_flag():
    """Verify try_parse fails when a required flag is missing."""
    args.string("auth-token", required = True)
    res = args.try_parse()
    assert(not res.ok, "try_parse should fail on missing required flag")
    assert("missing required flag: --auth-token" in res.error, "error message should name the flag")

def test_args_try_parse_missing_required_positional():
    """Verify try_parse fails when a required positional is missing."""
    args.positional("manifest-file", required = True)
    res = args.try_parse()
    assert(not res.ok, "try_parse should fail on missing required positional")
    assert("missing required positional argument: <manifest-file>" in res.error, "error message should name positional")
