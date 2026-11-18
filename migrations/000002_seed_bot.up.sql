-- Seed bot mac dinh "@bot" va cac mau ban dau.
-- Them mau moi: INSERT thanh 1 migration moi (khong sua file nay).

INSERT INTO bots (id, slug, name, description)
VALUES (
    '00000000-0000-0000-0000-0000000000b0',
    'bot',
    'Workspace Bot',
    'Nhan mau yeu cau / phieu sua trong chat va luu lai'
)
ON CONFLICT (slug) DO NOTHING;

INSERT INTO bot_templates (bot_id, code, name, description, schema)
VALUES
(
    '00000000-0000-0000-0000-0000000000b0',
    'yeu_cau',
    'Tao yeu cau',
    'Tao yeu cau thiet ke trong nhom chat',
    '{"fields":[
        {"key":"tieu_de","label":"Tieu de","type":"text","required":true},
        {"key":"to","label":"To/Bo phan","type":"text","required":true},
        {"key":"so_luong","label":"So luong","type":"number","required":true},
        {"key":"dinh_muc","label":"Dinh muc","type":"text","required":false},
        {"key":"nguoi_phu_trach","label":"Nguoi phu trach","type":"user","required":false},
        {"key":"deadline","label":"Deadline","type":"date","required":false},
        {"key":"ghi_chu","label":"Ghi chu","type":"textarea","required":false}
    ]}'::jsonb
),
(
    '00000000-0000-0000-0000-0000000000b0',
    'phieu_sua',
    'Phieu sua',
    'Ghi nhan phieu sua cho mot task',
    '{"fields":[
        {"key":"ma_task","label":"Ma task","type":"text","required":true},
        {"key":"nguyen_nhan","label":"Nguyen nhan","type":"textarea","required":true},
        {"key":"loai_sua","label":"Loai sua","type":"text","required":true},
        {"key":"so_luong","label":"So luong","type":"number","required":true},
        {"key":"ghi_chu","label":"Ghi chu","type":"textarea","required":false}
    ]}'::jsonb
)
ON CONFLICT (bot_id, code) DO NOTHING;
