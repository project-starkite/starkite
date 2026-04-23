# retry_test.star - Tests for retry module

# --- retry.do tests ---

def test_do_success_first():
    """func returns Result(ok=True) → ok, attempts=1"""
    def succeeds():
        return Result(ok=True, value="done")

    result = retry.do(succeeds, delay="1ms")
    assert(result.ok == True, "should be ok")
    assert(result.attempts == 1, "should be 1 attempt, got " + str(result.attempts))
    assert(result.value == "done", "value should be done")

def test_do_success_after_retries():
    """fail 2x then succeed → ok, attempts=3"""
    counter = {"n": 0}
    def sometimes_fails():
        counter["n"] += 1
        if counter["n"] <= 2:
            return Result(ok=False, error="not yet")
        return Result(ok=True, value="finally")

    result = retry.do(sometimes_fails, max_attempts=5, delay="1ms")
    assert(result.ok == True, "should succeed eventually")
    assert(result.attempts == 3, "should be 3 attempts, got " + str(result.attempts))
    assert(result.value == "finally", "value should be finally")

def test_do_all_fail():
    """always fail → not ok, attempts=max"""
    def always_fails():
        return Result(ok=False, error="nope")

    result = retry.do(always_fails, max_attempts=3, delay="1ms")
    assert(result.ok == False, "should fail")
    assert(result.attempts == 3, "should exhaust all attempts")
    assert(result.error == "nope", "should have last error")

def test_do_custom_max_attempts():
    """max_attempts=5 honored"""
    def fails():
        return Result(ok=False, error="fail")

    result = retry.do(fails, max_attempts=5, delay="1ms")
    assert(result.attempts == 5, "should make 5 attempts, got " + str(result.attempts))

def test_do_custom_delay():
    """delay="100ms" accepted"""
    def succeeds():
        return Result(ok=True, value="ok")

    result = retry.do(succeeds, max_attempts=3, delay="10ms")
    assert(result.ok == True, "should succeed")

def test_do_none_is_success():
    """func returns None → success, no retry"""
    def returns_none():
        return None

    result = retry.do(returns_none, max_attempts=3, delay="1ms")
    assert(result.ok == True, "None should be success")
    assert(result.attempts == 1, "should not retry on None")

def test_do_bool_true_success():
    """func returns True → success"""
    def returns_true():
        return True

    result = retry.do(returns_true, delay="1ms")
    assert(result.ok == True, "True should be success")
    assert(result.attempts == 1, "should not retry on True")

def test_do_bool_false_retries():
    """func returns False → failure, retries"""
    def returns_false():
        return False

    result = retry.do(returns_false, max_attempts=2, delay="1ms")
    assert(result.ok == False, "False should be failure")
    assert(result.attempts == 2, "should retry on False")

def test_do_non_result_no_retry():
    """func returns "hello" → execute once, value="hello"""
    def returns_string():
        return "hello"

    result = retry.do(returns_string, max_attempts=5, delay="1ms")
    assert(result.ok == True, "non-Result should be treated as success")
    assert(result.attempts == 1, "should execute once, not retry")
    assert(result.value == "hello", "should preserve the value")

def test_do_result_type():
    """return type is RetryResult"""
    def succeeds():
        return Result(ok=True, value="x")

    result = retry.do(succeeds, delay="1ms")
    assert(type(result) == "RetryResult", "should be RetryResult, got " + type(result))

def test_do_retry_result_attrs():
    """.ok, .value, .error, .attempts, .elapsed, .errors all work"""
    counter = {"n": 0}
    def fails_once():
        counter["n"] += 1
        if counter["n"] <= 1:
            return Result(ok=False, error="first fail")
        return Result(ok=True, value="success")

    result = retry.do(fails_once, max_attempts=3, delay="1ms")
    assert(result.ok == True, "should succeed")
    assert(result.value == "success", "value should be success")
    assert(result.error == "", "error should be empty on success")
    assert(result.attempts == 2, "should be 2 attempts")
    assert(type(result.elapsed) == "string", "elapsed should be string")
    assert(type(result.errors) == "list", "errors should be list")

def test_do_errors_list():
    """.errors contains error from each failed attempt"""
    def fails():
        return Result(ok=False, error="oops")

    result = retry.do(fails, max_attempts=3, delay="1ms")
    assert(len(result.errors) == 3, "should have 3 errors, got " + str(len(result.errors)))
    assert(result.errors[0] == "oops", "first error should be oops")

def test_do_retry_on_predicate():
    """retry_on=callable controls continuation"""
    counter = {"n": 0}
    def my_func():
        counter["n"] += 1
        if counter["n"] <= 2:
            return Result(ok=True, value="not ready")
        return Result(ok=True, value="ready")

    def should_retry(val):
        return val.value == "not ready"

    result = retry.do(my_func, max_attempts=5, delay="1ms", retry_on=should_retry)
    assert(result.ok == True, "should succeed")
    assert(result.value == "ready", "should get final value")
    assert(result.attempts == 3, "should take 3 attempts, got " + str(result.attempts))

def test_do_on_retry_callback():
    """on_retry called on each retry"""
    retries = {"count": 0}
    def fails():
        return Result(ok=False, error="fail")

    def on_retry(attempt, error):
        retries["count"] += 1

    result = retry.do(fails, max_attempts=3, delay="1ms", on_retry=on_retry)
    assert(retries["count"] == 2, "on_retry should be called twice (not on last attempt), got " + str(retries["count"]))

# --- retry.with_backoff tests ---

def test_backoff_success():
    """succeeds after retries"""
    counter = {"n": 0}
    def sometimes_fails():
        counter["n"] += 1
        if counter["n"] <= 2:
            return Result(ok=False, error="not yet")
        return Result(ok=True, value="done")

    result = retry.with_backoff(sometimes_fails, max_attempts=5, delay="1ms", max_delay="10ms", jitter=False)
    assert(result.ok == True, "should succeed")
    assert(result.attempts == 3, "should take 3 attempts")

def test_backoff_all_fail():
    """exhausts attempts"""
    def fails():
        return Result(ok=False, error="fail")

    result = retry.with_backoff(fails, max_attempts=3, delay="1ms", max_delay="10ms", jitter=False)
    assert(result.ok == False, "should fail")
    assert(result.attempts == 3, "should exhaust all attempts")

def test_backoff_max_delay():
    """max_delay caps growth"""
    def fails():
        return Result(ok=False, error="fail")

    result = retry.with_backoff(fails, max_attempts=3, delay="1ms", max_delay="5ms", jitter=False)
    assert(result.ok == False, "should fail")

def test_backoff_custom_params():
    """max_attempts + delay + max_delay"""
    def succeeds():
        return Result(ok=True, value="ok")

    result = retry.with_backoff(succeeds, max_attempts=10, delay="1ms", max_delay="100ms")
    assert(result.ok == True, "should succeed")
    assert(result.attempts == 1, "should succeed first try")

# --- Result builtin tests ---

def test_result_builtin_ok():
    """Result(ok=True, value="x") → .ok == True"""
    r = Result(ok=True, value="x")
    assert(r.ok == True, "should be ok")
    assert(r.value == "x", "value should be x")

def test_result_builtin_fail():
    """Result(ok=False, error="e") → .ok == False"""
    r = Result(ok=False, error="e")
    assert(r.ok == False, "should not be ok")
    assert(r.error == "e", "error should be e")

def test_result_builtin_truth():
    """if Result(ok=True): passes"""
    r = Result(ok=True, value="test")
    passed = False
    if r:
        passed = True
    assert(passed, "Result(ok=True) should be truthy")

    r2 = Result(ok=False, error="err")
    passed2 = True
    if r2:
        passed2 = False
    assert(passed2 == True, "Result(ok=False) should be falsy (should not enter if)")

def test_result_with_retry():
    """func uses Result() builtin, retry.do evaluates it"""
    counter = {"n": 0}
    def check_service():
        counter["n"] += 1
        if counter["n"] < 3:
            return Result(ok=False, error="service down")
        return Result(ok=True, value="healthy")

    result = retry.do(check_service, max_attempts=5, delay="1ms")
    assert(result.ok == True, "should succeed")
    assert(result.value == "healthy", "should get healthy")
    assert(result.attempts == 3, "should take 3 attempts")

# --- edge cases ---

def test_do_go_error():
    """func raises Starlark error → captured in errors list"""
    def raises_error():
        fail("something broke")

    result = retry.do(raises_error, max_attempts=2, delay="1ms")
    assert(result.ok == False, "should fail")
    assert(len(result.errors) == 2, "should capture errors from both attempts")

def test_retry_result_is_truthy():
    """RetryResult(ok=True) is truthy in `if result:`"""
    def succeeds():
        return Result(ok=True, value="yes")

    result = retry.do(succeeds, delay="1ms")
    passed = False
    if result:
        passed = True
    assert(passed, "truthy RetryResult should pass if check")
