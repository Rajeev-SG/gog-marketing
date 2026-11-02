import { Link } from "react-aria-components";
import { PRODUCT_NAVIGATION, type ProductNavId } from "../product-navigation.js";
import { Icon } from "./index.js";

/**
 * The three application destinations, shared by the workspace and chat (L09).
 * A count badge marks a destination that needs attention; its text is part of
 * the link name so screen readers hear it too.
 */
export function ProductNav({
  active,
  label,
  className,
  badges = {},
}: {
  active: ProductNavId;
  label: string;
  className: string;
  badges?: Partial<Record<ProductNavId, number>>;
}) {
  return (
    <nav className={className} aria-label={label}>
      {PRODUCT_NAVIGATION.map((item) => {
        const count = badges[item.id] ?? 0;
        return (
          <Link
            key={item.id}
            href={item.href}
            aria-current={active === item.id ? "page" : undefined}
            data-product-nav={item.id}
          >
            <Icon path={item.path} />
            <span>{item.label}</span>
            {count > 0 && (
              <span className="nav-badge">
                {count}
                <span className="sr-only">
                  {" "}
                  {count === 1 ? "item needs" : "items need"} attention
                </span>
              </span>
            )}
          </Link>
        );
      })}
    </nav>
  );
}
