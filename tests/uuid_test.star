# uuid_test.star - Tests for uuid module

def test_v4():
    """Test uuid.v4 generates valid UUID."""
    id = uuid.v4()
    assert(len(id) == 36, "UUID should be 36 chars")
    assert(id.count("-") == 4, "UUID should have 4 dashes")

def test_v4_unique():
    """Test uuid.v4 generates unique values."""
    id1 = uuid.v4()
    id2 = uuid.v4()
    id3 = uuid.v4()
    assert(id1 != id2, "UUIDs should be unique")
    assert(id2 != id3, "UUIDs should be unique")
    assert(id1 != id3, "UUIDs should be unique")

def test_v4_format():
    """Test uuid.v4 has correct format."""
    id = uuid.v4()
    parts = id.split("-")
    assert(len(parts) == 5, "UUID should have 5 parts")
    assert(len(parts[0]) == 8, "first part should be 8 chars")
    assert(len(parts[1]) == 4, "second part should be 4 chars")
    assert(len(parts[2]) == 4, "third part should be 4 chars")
    assert(len(parts[3]) == 4, "fourth part should be 4 chars")
    assert(len(parts[4]) == 12, "fifth part should be 12 chars")

def test_v4_version_char():
    """Test uuid.v4 has version 4 indicator."""
    id = uuid.v4()
    # Version 4 UUIDs have '4' as the first char of the third group
    parts = id.split("-")
    assert(parts[2][0] == "4", "version should be 4")

def test_v4_lowercase():
    """Test uuid.v4 uses lowercase."""
    id = uuid.v4()
    assert(id == id.lower(), "UUID should be lowercase")

def test_multiple_generation():
    """Test generating many UUIDs."""
    ids = {}
    for i in range(100):
        id = uuid.v4()
        assert(id not in ids, "all UUIDs should be unique")
        ids[id] = True
