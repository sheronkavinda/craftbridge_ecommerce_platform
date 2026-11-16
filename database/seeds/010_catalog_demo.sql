
-- CraftBridge Sample Categories

INSERT INTO categories
(name, slug, description)
VALUES
(
    'Jewelry',
    'jewelry',
    'Handmade jewelry and accessories'
),
(
    'Home Decor',
    'home-decor',
    'Handmade decorations for homes'
),
(
    'Textiles',
    'textiles',
    'Handwoven and handcrafted fabrics'
),
(
    'Art',
    'art',
    'Original handmade artwork'
),
(
    'Gifts',
    'gifts',
    'Unique handmade gift items'
),
(
    'Pottery',
    'pottery',
    'Handcrafted ceramic products'
)
ON CONFLICT (slug) DO NOTHING;
