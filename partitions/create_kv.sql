create table kv (
    pk text not null,
    sk text not null,
    data text not null,

    _created_at timestamp not null default CURRENT_TIMESTAMP,
    _updated_at timestamp not null default CURRENT_TIMESTAMP,

    primary key(pk, sk)
);