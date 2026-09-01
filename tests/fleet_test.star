# fleet_test.star - Comprehensive test suite for fleet module

# ============================================================================
# Constructors
# ============================================================================

def test_fleet_new_list_of_dicts():
    f = fleet.new([
        {"name": "web-1", "address": "10.0.1.10", "role": "web", "zone": "us-east-1a"},
        {"name": "web-2", "address": "10.0.1.11", "role": "web", "zone": "us-east-1b"},
        {"name": "db-1", "address": "10.0.2.10", "role": "db", "zone": "us-east-1a"},
    ])
    assert(type(f) == "fleet", "expected fleet type")
    assert(f.count == 3, "expected 3 items in fleet")
    assert(len(f.items) == 3, "expected 3 items in f.items")

def test_fleet_new_keyword_list():
    f = fleet.new(list=[
        {"name": "srv-1", "address": "192.168.1.1"},
        {"name": "srv-2", "address": "192.168.1.2"},
    ])
    assert(f.count == 2, "expected 2 items")

def test_fleet_new_list_of_strings():
    f = fleet.new([
        "192.168.1.101",
        "192.168.1.102",
        "192.168.1.103",
    ])
    assert(f.count == 3, "expected 3 items from string list")
    addrs = f.addresses()
    assert(addrs[0] == "192.168.1.101", "first address should match")
    assert(addrs[1] == "192.168.1.102", "second address should match")
    assert(addrs[2] == "192.168.1.103", "third address should match")
    assert(f.names()[0] == "192.168.1.101", "name should default to address")

def test_fleet_new_callable():
    def discover():
        return [
            {"name": "api-1", "address": "172.16.0.1", "tier": "backend"},
            {"name": "api-2", "address": "172.16.0.2", "tier": "backend"},
        ]
    
    f = fleet.new(function=discover)
    assert(f.count == 2, "expected 2 items from callable")
    assert(f.first()["name"] == "api-1", "expected first name api-1")
    assert(f.first()["tier"] == "backend", "expected tier label backend")

def test_fleet_new_json_keyword():
    json_payload = '[{"name": "cache-1", "address": "10.10.1.1", "role": "cache"}]'
    f = fleet.new(json=json_payload)
    assert(f.count == 1, "expected 1 item from json string")
    assert(f.first()["name"] == "cache-1", "expected cache-1")
    assert(f.first()["role"] == "cache", "expected role cache")

def test_fleet_file_yaml():
    tmp_path = "/tmp/starkite_test_fleet.yaml"
    yaml_content = """
- name: node-alpha
  address: 10.0.0.1
  role: control-plane
  env: production
- name: node-beta
  address: 10.0.0.2
  role: worker
  env: production
- name: node-gamma
  address: 10.0.0.3
  role: worker
  env: staging
"""
    write_text(tmp_path, yaml_content)
    
    f = fleet.file(tmp_path)
    assert(f.count == 3, "expected 3 nodes in file")
    
    prod_workers = f.filter(role="worker", env="production")
    assert(prod_workers.count == 1, "expected 1 prod worker")
    assert(prod_workers.first()["name"] == "node-beta", "expected node-beta")

    # Also test via fleet.new(file=...)
    f_kw = fleet.new(file=tmp_path)
    assert(f_kw.count == 3, "expected 3 nodes from fleet.new(file=...)")

def test_fleet_hosts_file_posix():
    tmp_hosts = "/tmp/starkite_test_hosts"
    hosts_content = """
# /etc/hosts format
127.0.0.1 localhost localhost.localdomain
::1       localhost6

192.168.10.100 picluster-0 picluster-0.local master
192.168.10.101 picluster-1 picluster-1.local worker-1
192.168.10.102 picluster-2 picluster-2.local worker-2
"""
    write_text(tmp_hosts, hosts_content)

    # 1. By default, loopback entries are excluded
    f = fleet.hosts_file(tmp_hosts)
    assert(f.count == 3, "expected 3 cluster nodes, got " + str(f.count))
    assert(f.names() == ["picluster-0", "picluster-1", "picluster-2"], "expected hostnames")
    assert(f.addresses() == ["192.168.10.100", "192.168.10.101", "192.168.10.102"], "expected addresses")
    assert(f.first()["master"] == "true", "expected master alias indexed in labels")

    # 2. Including loopback
    f_all = fleet.hosts_file(tmp_hosts, loopback=True)
    assert(f_all.count == 5, "expected 5 total hosts including loopback")

    # 3. Via fleet.host_file alias
    f_alias = fleet.host_file(tmp_hosts)
    assert(f_alias.count == 3, "expected 3 nodes via host_file")

    # 4. Via fleet.new(hosts_file=...)
    f_new_kw = fleet.new(hosts_file=tmp_hosts)
    assert(f_new_kw.count == 3, "expected 3 nodes via fleet.new(hosts_file=...)")

# ============================================================================
# Querying & Subsetting
# ============================================================================

def test_fleet_filter_keywords():
    f = fleet.new([
        {"name": "web-1", "address": "10.0.1.10", "role": "web", "env": "prod"},
        {"name": "web-2", "address": "10.0.1.11", "role": "web", "env": "stage"},
        {"name": "db-1", "address": "10.0.2.10", "role": "db", "env": "prod"},
    ])
    
    # Filter by role
    web_nodes = f.filter(role="web")
    assert(web_nodes.count == 2, "expected 2 web nodes")
    
    # Filter by role and env
    prod_web = f.filter(role="web", env="prod")
    assert(prod_web.count == 1, "expected 1 prod web node")
    assert(prod_web.first()["name"] == "web-1", "expected web-1")

def test_fleet_filter_predicate_lambda():
    f = fleet.new([
        {"name": "s1", "address": "10.0.1.1", "cpu": 4, "mem": 16},
        {"name": "s2", "address": "10.0.1.2", "cpu": 8, "mem": 32},
        {"name": "s3", "address": "10.0.1.3", "cpu": 16, "mem": 64},
    ])
    
    high_perf = f.filter(lambda node: node.get("cpu", 0) >= 8)
    assert(high_perf.count == 2, "expected 2 high perf nodes")
    assert(high_perf.names() == ["s2", "s3"], "expected s2 and s3")

def test_fleet_group_by():
    f = fleet.new([
        {"name": "web-1", "address": "10.0.1.10", "role": "web"},
        {"name": "web-2", "address": "10.0.1.11", "role": "web"},
        {"name": "db-1", "address": "10.0.2.10", "role": "db"},
    ])
    
    groups = f.group_by("role")
    assert(type(groups) == "dict", "expected dict from group_by")
    assert(len(groups) == 2, "expected 2 groups")
    assert("web" in groups, "expected web group")
    assert("db" in groups, "expected db group")
    
    web_fleet = groups["web"]
    assert(web_fleet.count == 2, "expected 2 in web fleet")
    assert(groups["db"].count == 1, "expected 1 in db fleet")

# ============================================================================
# Extraction Methods
# ============================================================================

def test_fleet_extraction():
    f = fleet.new([
        {"id": "id-1", "name": "srv-1", "address": "10.0.1.1", "private_ip": "172.16.1.1"},
        {"id": "id-2", "name": "srv-2", "address": "10.0.1.2", "private_ip": "172.16.1.2"},
    ])
    
    assert(f.names() == ["srv-1", "srv-2"], "expected srv-1 and srv-2")
    assert(f.ids() == ["id-1", "id-2"], "expected id-1 and id-2")
    assert(f.addresses() == ["10.0.1.1", "10.0.1.2"], "expected default addresses")
    assert(f.addresses(key="private_ip") == ["172.16.1.1", "172.16.1.2"], "expected custom private_ip")

def test_fleet_first_empty():
    f = fleet.new([])
    assert(f.count == 0, "expected 0 count")
    assert(f.first() == None, "first() on empty fleet should return None")
    assert(not f, "empty fleet should evaluate to False")

# ============================================================================
# SSH Integration
# ============================================================================

def test_ssh_config_with_fleet():
    f = fleet.new([
        {"name": "host1", "address": "10.0.1.10", "role": "web"},
        {"name": "host2", "address": "10.0.1.11", "role": "web"},
    ])
    
    client = ssh.config(fleet=f, user="deploy", dry_run=True)
    assert(type(client) == "ssh.client", "expected ssh.client")
    assert(client.hosts == ["10.0.1.10", "10.0.1.11"], "expected hosts extracted from fleet")
    assert(client.fleet != None, "client.fleet should be populated")
    assert(client.fleet.count == 2, "client.fleet count should be 2")
    
    results = client.exec("uptime")
    assert(len(results) == 2, "expected 2 results")
    assert(results[0].host == "10.0.1.10", "first host should match")
    assert(results[1].host == "10.0.1.11", "second host should match")

def test_ssh_config_with_hosts_shortcut():
    client = ssh.config(hosts=["192.168.1.50", "192.168.1.51"], user="root", dry_run=True)
    assert(client.hosts == ["192.168.1.50", "192.168.1.51"], "expected hosts")
    assert(client.fleet != None, "fleet should be synthesized from hosts shortcut")
    assert(client.fleet.count == 2, "synthesized fleet count should be 2")

# ============================================================================
# Try Module Variants
# ============================================================================

def test_fleet_try_file():
    res = fleet.try_file("/nonexistent/file/path.yaml")
    assert(res.ok == False, "try_file on nonexistent path should fail gracefully")
    assert(res.error != "", "error message should be populated")
