DROP TABLE IF EXISTS usuario_equipes;
ALTER TABLE usuarios DROP COLUMN IF EXISTS auth_provider;
ALTER TABLE usuarios ALTER COLUMN senha_hash SET NOT NULL;
