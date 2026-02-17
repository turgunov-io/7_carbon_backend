-- carbon_go bootstrap schema
-- Run this file once in a clean DB (or repeatedly; all CREATE statements are idempotent).
-- Active API routes in main.go: /banners, /portfolio_items, /work_post

BEGIN;

-- Optional but explicit.
CREATE SCHEMA IF NOT EXISTS public;

-- 1. Navbar
CREATE TABLE IF NOT EXISTS public.navbar_items (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    position INTEGER NOT NULL DEFAULT 0
);

-- 2. Banners (used by GET /banners)
CREATE TABLE IF NOT EXISTS public.banners (
    id SERIAL PRIMARY KEY,
    section TEXT NOT NULL,
    title TEXT NOT NULL,
    image_url TEXT NOT NULL,
    priority INTEGER NOT NULL DEFAULT 0
);

-- 3. Services taxonomy
CREATE TABLE IF NOT EXISTS public.service_groups (
    id SERIAL PRIMARY KEY,
    title TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS public.service_categories (
    id SERIAL PRIMARY KEY,
    group_id INTEGER REFERENCES public.service_groups(id) ON DELETE SET NULL,
    external_id TEXT UNIQUE,
    slug TEXT UNIQUE,
    title TEXT NOT NULL,
    description TEXT,
    image_url TEXT,
    position INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS public.services (
    id SERIAL PRIMARY KEY,
    category_id INTEGER REFERENCES public.service_categories(id) ON DELETE SET NULL,
    title TEXT NOT NULL,
    description_html TEXT,
    icon_url TEXT,
    position INTEGER NOT NULL DEFAULT 0
);

-- 4. Why us
CREATE TABLE IF NOT EXISTS public.why_uct (
    id SMALLINT PRIMARY KEY DEFAULT 1,
    title TEXT NOT NULL,
    description_html TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS public.why_uct_items (
    id SERIAL PRIMARY KEY,
    why_uct_id SMALLINT NOT NULL DEFAULT 1 REFERENCES public.why_uct(id) ON DELETE CASCADE,
    icon_url TEXT,
    title TEXT NOT NULL,
    description_html TEXT NOT NULL,
    position INTEGER NOT NULL DEFAULT 0
);

-- 5. Portfolio (used by GET /portfolio_items)
CREATE TABLE IF NOT EXISTS public.portfolio_items (
    id SERIAL PRIMARY KEY,
    brand TEXT,
    title TEXT NOT NULL,
    image_url TEXT NOT NULL,
    description TEXT,
    youtube_link TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 6. Partners
CREATE TABLE IF NOT EXISTS public.partners (
    id SERIAL PRIMARY KEY,
    name TEXT,
    logo_url TEXT NOT NULL,
    position INTEGER NOT NULL DEFAULT 0
);

-- 7. Footer
CREATE TABLE IF NOT EXISTS public.footer_contacts (
    id SERIAL PRIMARY KEY,
    phone_number TEXT,
    email TEXT,
    logo_svg_url TEXT
);

CREATE TABLE IF NOT EXISTS public.footer_addresses (
    id SERIAL PRIMARY KEY,
    contact_id INTEGER REFERENCES public.footer_contacts(id) ON DELETE CASCADE,
    address TEXT NOT NULL,
    city TEXT,
    country TEXT,
    position INTEGER NOT NULL DEFAULT 0
);

-- 8. Tuning page
CREATE TABLE IF NOT EXISTS public.tuning_page (
    id SMALLINT PRIMARY KEY DEFAULT 1,
    intro_description TEXT,
    extra_description TEXT
);

CREATE TABLE IF NOT EXISTS public.tuning_cards (
    id SERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    image_url TEXT NOT NULL,
    position INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS public.tuning_description_images (
    id SERIAL PRIMARY KEY,
    image_url TEXT NOT NULL,
    caption TEXT,
    position INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS public.tuning_dynamic_metrics (
    id SERIAL PRIMARY KEY,
    description TEXT,
    image_url TEXT,
    speed NUMERIC(6,2)
);

CREATE TABLE IF NOT EXISTS public.tuning_measurement_charts (
    id SERIAL PRIMARY KEY,
    description TEXT,
    image_url TEXT
);

-- 9. About page
CREATE TABLE IF NOT EXISTS public.about_page (
    id SMALLINT PRIMARY KEY DEFAULT 1,
    banner_image_url TEXT,
    banner_title TEXT,
    history_description TEXT,
    video_url TEXT,
    mission_description TEXT,
    mission_image_url TEXT
);

CREATE TABLE IF NOT EXISTS public.about_development_milestones (
    id SERIAL PRIMARY KEY,
    about_id SMALLINT NOT NULL DEFAULT 1 REFERENCES public.about_page(id) ON DELETE CASCADE,
    image_url TEXT,
    description TEXT,
    year INTEGER,
    position INTEGER NOT NULL DEFAULT 0
);

-- 10. Contact page
CREATE TABLE IF NOT EXISTS public.contact_page (
    id SMALLINT PRIMARY KEY DEFAULT 1,
    phone_number TEXT,
    address TEXT,
    description TEXT,
    image_url TEXT
);

-- 10.1 Contact (used by GET /contact when table exists)
CREATE TABLE IF NOT EXISTS public.contact (
    id SERIAL PRIMARY KEY,
    phone_number TEXT,
    address TEXT,
    description TEXT,
    email TEXT,
    work_schedule TEXT
);

-- 11. Consultation leads
CREATE TABLE IF NOT EXISTS public.consultation_leads (
    id BIGSERIAL PRIMARY KEY,
    name TEXT,
    phone_number TEXT,
    email TEXT,
    marka TEXT,
    brand TEXT,
    service_name TEXT,
    comments TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 12. Privacy policy
CREATE TABLE IF NOT EXISTS public.privacy_sections (
    id SERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    position INTEGER NOT NULL DEFAULT 0
);

-- 13. Certificates
CREATE TABLE IF NOT EXISTS public.certificates (
    id SERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT,
    image_url TEXT NOT NULL,
    position INTEGER NOT NULL DEFAULT 0
);

-- 14. External widget config
CREATE TABLE IF NOT EXISTS public.widget_configs (
    id SMALLINT PRIMARY KEY DEFAULT 1,
    provider TEXT,
    script_url TEXT,
    embed_code TEXT
);

-- 15. Work posts (used by GET /work_post)
-- Backend first tries public.work_post, then falls back to public.blog_posts.
CREATE TABLE IF NOT EXISTS public.work_post (
    id SERIAL PRIMARY KEY,
    title_model TEXT NOT NULL,
    card_image_url TEXT,
    full_image_url TEXT,
    card_description TEXT,
    work_list JSONB,
    gallery_images JSONB,
    full_description TEXT,
    video_image_url TEXT,
    video_link TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Legacy compatibility table.
CREATE TABLE IF NOT EXISTS public.blog_posts (
    id SERIAL PRIMARY KEY,
    title_model TEXT NOT NULL,
    card_image_url TEXT,
    full_image_url TEXT,
    card_description TEXT,
    work_list JSONB,
    gallery_images JSONB,
    full_description TEXT,
    video_image_url TEXT,
    video_link TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Ensure compatibility for already existing databases.
ALTER TABLE IF EXISTS public.work_post
    ADD COLUMN IF NOT EXISTS gallery_images JSONB;

ALTER TABLE IF EXISTS public.blog_posts
    ADD COLUMN IF NOT EXISTS gallery_images JSONB;

-- 16. Shop
CREATE TABLE IF NOT EXISTS public.shop_developers (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS public.shop_models (
    id SERIAL PRIMARY KEY,
    developer_id INTEGER REFERENCES public.shop_developers(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT,
    video_image_url TEXT,
    video_link TEXT
);

CREATE TABLE IF NOT EXISTS public.shop_model_images (
    id SERIAL PRIMARY KEY,
    model_id INTEGER REFERENCES public.shop_models(id) ON DELETE CASCADE,
    image_url TEXT NOT NULL,
    position INTEGER NOT NULL DEFAULT 0
);

-- Indexes for active API sort patterns.
CREATE INDEX IF NOT EXISTS idx_banners_priority_id
    ON public.banners (priority, id);

CREATE INDEX IF NOT EXISTS idx_portfolio_items_created_at_id
    ON public.portfolio_items (created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_work_post_created_at_id
    ON public.work_post (created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_blog_posts_created_at_id
    ON public.blog_posts (created_at DESC, id DESC);

-- Seed data for active routes (insert only when table is empty).
INSERT INTO public.banners (section, title, image_url, priority)
SELECT 'home', 'Main banner', 'https://example.com/banner-1.jpg', 1
WHERE NOT EXISTS (SELECT 1 FROM public.banners);

INSERT INTO public.portfolio_items (brand, title, image_url, description, youtube_link)
SELECT
    'BMW',
    'Demo portfolio item',
    'https://example.com/portfolio-1.jpg',
    'Demo description',
    NULL
WHERE NOT EXISTS (SELECT 1 FROM public.portfolio_items);

INSERT INTO public.work_post (
    title_model,
    card_image_url,
    full_image_url,
    card_description,
    work_list,
    gallery_images,
    full_description,
    video_image_url,
    video_link
)
SELECT
    'Tesla Model 3',
    'https://example.com/work-card.jpg',
    'https://example.com/work-full.jpg',
    'Power and throttle response upgrade',
    '[{"step":"Diagnostics"},{"step":"Calibration"},{"step":"Road test"}]'::jsonb,
    '["https://example.com/work-1.jpg","https://example.com/work-2.jpg","https://example.com/work-3.jpg"]'::jsonb,
    'Stage 1 tuning with stable daily setup.',
    'https://example.com/work-video-cover.jpg',
    'https://www.youtube.com/watch?v=dQw4w9WgXcQ'
WHERE NOT EXISTS (SELECT 1 FROM public.work_post);

COMMIT;

-- Optional checks after execution:
-- SELECT COUNT(*) FROM public.banners;
-- SELECT COUNT(*) FROM public.portfolio_items;
-- SELECT COUNT(*) FROM public.work_post;
