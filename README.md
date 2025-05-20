# Ycsb Go with Prometheus metrics

Original: https://github.com/pingcap/octopus/tree/master/ycsb

Difference from original:

- Removed all the non tidb/sql support (tikv, raw, coprocessor)
- Removed Prometheus push functionality
- Read DB name from the connection DNS/URL (and remove hardcoded DB name), also remove DB creation code
