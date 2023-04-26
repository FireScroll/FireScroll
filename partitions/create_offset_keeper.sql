create table offset_keeper (
    id int not null,
    t timestamp not null,

    _updated_at timestamp not null default CURRENT_TIMESTAMP,

    primary key(id)
);