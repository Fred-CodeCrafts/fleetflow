-- API keys for the public ingest endpoints (DESIGN 3.5, 3.7; PRD US-13).
--
-- A key is stored only as the SHA-256 digest of its plaintext. The plaintext is
-- generated once, shown to its owner once, and never written here, so a dump of
-- this table yields nothing a caller could present.
--
-- There is no pepper and no per-key salt: a key is 256 bits of uniform
-- randomness, so there is nothing to brute-force offline, and the digest has to
-- stay deterministic because it is the lookup key. That lookup is why the hash is
-- unique and indexed: verification is one indexed read rather than a scan, and it
-- never depends on how many bytes of a wrong key happened to match.

create table api_keys (
    id         text primary key,
    name       text not null,
    prefix     text not null,
    hash       bytea not null,
    created_at timestamptz not null default now(),

    -- The name is the only way a human tells two keys apart in a list, so a key
    -- without one is not auditable.
    constraint api_keys_name_not_blank check (btrim(name) <> ''),
    -- SHA-256 is 32 octets; another length is not a digest this code wrote.
    constraint api_keys_hash_is_sha256 check (octet_length(hash) = 32),
    -- Two records for one key would make verification ambiguous. The unique
    -- constraint also supplies the index the lookup by hash needs, so there is
    -- deliberately no second index on this column.
    constraint api_keys_hash_unique unique (hash)
);
