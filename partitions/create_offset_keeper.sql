create table offset_keeper (
    id int4 not null,
    offset int8 not null,

    _updated_at timestamp not null default CURRENT_TIMESTAMP,

    primary key(id)
);