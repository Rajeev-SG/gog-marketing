/**
 * gog-marketing component library: thin, token-styled wrappers over React Aria
 * Components. React Aria supplies keyboard, focus, screen-reader and touch
 * behaviour; ui.css supplies the look from product.css tokens.
 * Usage rules for each component live in docs/ui/component-catalogue.md.
 */
import { createContext, useCallback, useContext, useRef, useState, type ReactNode } from "react";
import {
  Button as AriaButton,
  Checkbox as AriaCheckbox,
  Dialog,
  Disclosure as AriaDisclosure,
  DisclosurePanel,
  Heading,
  Input,
  Label,
  Link as AriaLink,
  ListBox,
  ListBoxItem,
  Menu,
  MenuItem,
  MenuTrigger,
  Modal,
  ModalOverlay,
  Popover,
  SearchField as AriaSearchField,
  Select as AriaSelect,
  SelectValue,
  Header,
  Section,
  ToggleButton,
  ToggleButtonGroup,
  type ButtonProps as AriaButtonProps,
  type CheckboxProps as AriaCheckboxProps,
  type Key,
  type LinkProps as AriaLinkProps,
} from "react-aria-components";
import "./ui.css";

type Variant = "primary" | "secondary" | "danger" | "quiet" | "attention";

export interface ButtonProps extends Omit<AriaButtonProps, "className"> {
  variant?: Variant;
  size?: "default" | "small";
  className?: string;
}
/** Buttons perform actions. One primary per view; danger only for destructive actions. */
export function Button({
  variant = "secondary",
  size = "default",
  className = "",
  ...props
}: ButtonProps) {
  return <AriaButton {...props} className={`ui-button ${variant} ${size} ${className}`.trim()} />;
}

export interface LinkButtonProps extends Omit<AriaLinkProps, "className"> {
  variant?: Variant;
  className?: string;
}
/** Navigation that looks like a button (it goes somewhere, so it is a link). */
export function LinkButton({ variant = "secondary", className = "", ...props }: LinkButtonProps) {
  return <AriaLink {...props} className={`ui-button ${variant} ${className}`.trim()} />;
}

export interface CheckboxProps extends Omit<AriaCheckboxProps, "className" | "children"> {
  children?: ReactNode;
  description?: ReactNode;
  className?: string;
}
/** Checkboxes select items that take effect on Save (NN/g: checkbox, not toggle). */
export function Checkbox({ children, description, className = "", ...props }: CheckboxProps) {
  return (
    <AriaCheckbox {...props} className={`ui-checkbox ${className}`.trim()}>
      {({ isIndeterminate }) => (
        <>
          <span className="ui-checkbox-box" aria-hidden="true">
            <svg viewBox="0 0 18 18">
              {isIndeterminate ? <path d="M4 9h10" /> : <path d="m4 9.5 3.2 3L14 5.5" />}
            </svg>
          </span>
          {(children || description) && (
            <span className="ui-checkbox-text">
              {children && <span className="ui-checkbox-label">{children}</span>}
              {description && <span className="ui-checkbox-description">{description}</span>}
            </span>
          )}
        </>
      )}
    </AriaCheckbox>
  );
}

export function SearchField({
  label,
  value,
  onChange,
  placeholder,
  isDisabled,
  className = "",
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  isDisabled?: boolean;
  className?: string;
}) {
  return (
    <AriaSearchField
      aria-label={label}
      value={value}
      onChange={onChange}
      isDisabled={isDisabled}
      className={`ui-search ${className}`.trim()}
    >
      <Icon name="search" />
      <Input placeholder={placeholder} />
      <AriaButton className="ui-search-clear" aria-label={`Clear ${label.toLowerCase()}`}>
        <Icon name="x" />
      </AriaButton>
    </AriaSearchField>
  );
}

/** Mutually exclusive view filter (NN/g segmented control); never for settings. */
export function SegmentedControl<T extends string>({
  label,
  options,
  value,
  onChange,
  isDisabled,
}: {
  label: string;
  options: ReadonlyArray<{ id: T; label: string }>;
  value: T;
  onChange: (value: T) => void;
  isDisabled?: boolean;
}) {
  return (
    <ToggleButtonGroup
      aria-label={label}
      selectionMode="single"
      disallowEmptySelection
      selectedKeys={[value]}
      onSelectionChange={(keys) => {
        const [next] = [...keys] as Key[];
        if (next !== undefined) onChange(String(next) as T);
      }}
      isDisabled={isDisabled}
      className="ui-segmented"
    >
      {options.map((option) => (
        <ToggleButton key={option.id} id={option.id} className="ui-segment">
          {option.label}
        </ToggleButton>
      ))}
    </ToggleButtonGroup>
  );
}

export interface MenuAction {
  id: string;
  label: string;
  danger?: boolean;
  onAction: () => void;
}
/** Overflow menu for secondary actions on one object. The trigger names the object. */
export function ActionMenu({
  label,
  heading,
  actions,
}: {
  label: string;
  heading?: string;
  actions: MenuAction[];
}) {
  return (
    <MenuTrigger>
      <AriaButton className="ui-button quiet ui-icon-button" aria-label={label}>
        <Icon name="more" />
      </AriaButton>
      <Popover placement="bottom end" className="ui-popover">
        <Menu
          className="ui-menu"
          onAction={(key) => actions.find((action) => action.id === key)?.onAction()}
        >
          <Section>
            {heading && <Header className="ui-menu-header">{heading}</Header>}
            {actions.map((action) => (
              <MenuItem
                key={action.id}
                id={action.id}
                className={`ui-menu-item${action.danger ? " danger" : ""}`}
              >
                {action.label}
              </MenuItem>
            ))}
          </Section>
        </Menu>
      </Popover>
    </MenuTrigger>
  );
}

/** Progressive disclosure for secondary detail (NN/g); the summary must stand alone. */
export function Disclosure({
  title,
  meta,
  children,
  defaultExpanded,
  className = "",
}: {
  title: ReactNode;
  meta?: ReactNode;
  children: ReactNode;
  defaultExpanded?: boolean;
  className?: string;
}) {
  return (
    <AriaDisclosure
      defaultExpanded={defaultExpanded}
      className={`ui-disclosure ${className}`.trim()}
    >
      <Heading level={3} className="ui-disclosure-heading">
        <AriaButton slot="trigger" className="ui-disclosure-trigger">
          <Icon name="chevron" className="ui-disclosure-chevron" />
          <span className="ui-disclosure-title">{title}</span>
          {meta && <span className="ui-disclosure-meta">{meta}</span>}
        </AriaButton>
      </Heading>
      <DisclosurePanel className="ui-disclosure-panel">{children}</DisclosurePanel>
    </AriaDisclosure>
  );
}

export type StatusTone = "ok" | "needs_attention" | "unavailable" | "disconnected";
/** Status is shown only when it needs attention or adds meaning (DESIGN.md status rule). */
export function StatusBadge({ tone, children }: { tone: StatusTone; children: ReactNode }) {
  return <span className={`status ${tone}`}>{children}</span>;
}

/** Pick one option from a list (NN/g: a dropdown for 5+ options or long labels). */
export function Select<T extends string>({
  label,
  options,
  value,
  onChange,
  isDisabled,
  className = "",
}: {
  label: string;
  options: ReadonlyArray<{ id: T; label: string; description?: string }>;
  value: T | null;
  onChange: (value: T) => void;
  isDisabled?: boolean;
  className?: string;
}) {
  return (
    <AriaSelect
      selectedKey={value}
      onSelectionChange={(key) => key !== null && onChange(String(key) as T)}
      isDisabled={isDisabled}
      className={`ui-select ${className}`.trim()}
    >
      <Label className="ui-select-label">{label}</Label>
      <AriaButton className="ui-select-trigger">
        <SelectValue className="ui-select-value" />
        <Icon name="chevron" className="ui-select-chevron" />
      </AriaButton>
      <Popover placement="bottom start" className="ui-popover ui-select-popover">
        <ListBox className="ui-menu">
          {options.map((option) => (
            <ListBoxItem
              key={option.id}
              id={option.id}
              textValue={option.label}
              className="ui-menu-item ui-select-option"
            >
              <span>
                <span className="ui-select-option-label">{option.label}</span>
                {option.description && (
                  <span className="ui-select-option-description">{option.description}</span>
                )}
              </span>
            </ListBoxItem>
          ))}
        </ListBox>
      </Popover>
    </AriaSelect>
  );
}

/**
 * A modal dialog for a short task (rename, confirm with a field). Escape and the
 * Cancel button close it unless `isBusy`; focus returns to the trigger.
 */
export function ModalDialog({
  isOpen,
  onClose,
  title,
  children,
  isBusy,
  role = "dialog",
}: {
  isOpen: boolean;
  onClose: () => void;
  title: string;
  children: ReactNode;
  isBusy?: boolean;
  role?: "dialog" | "alertdialog";
}) {
  return (
    <ModalOverlay
      isOpen={isOpen}
      onOpenChange={(open) => !open && !isBusy && onClose()}
      isDismissable={!isBusy}
      isKeyboardDismissDisabled={isBusy}
      className="ui-modal-overlay"
    >
      <Modal className="ui-modal">
        <Dialog role={role} className="ui-dialog">
          <Heading slot="title" className="ui-dialog-title">
            {title}
          </Heading>
          {children}
        </Dialog>
      </Modal>
    </ModalOverlay>
  );
}

// ---- Confirmation dialogs (replace window.confirm) ---------------------------------

export interface ConfirmOptions {
  title: string;
  body: ReactNode;
  confirmLabel: string;
  cancelLabel?: string;
  destructive?: boolean;
}
type ConfirmFn = (options: ConfirmOptions) => Promise<boolean>;
const ConfirmContext = createContext<ConfirmFn | null>(null);

/**
 * Alert dialogs for consequential choices (NN/g: confirm only what is hard to undo).
 * The safe action is first and receives focus; Escape cancels.
 */
export function ConfirmProvider({ children }: { children: ReactNode }) {
  const [pending, setPending] = useState<ConfirmOptions | null>(null);
  const resolver = useRef<((value: boolean) => void) | null>(null);
  const confirm = useCallback<ConfirmFn>((options) => {
    resolver.current?.(false);
    setPending(options);
    return new Promise<boolean>((resolve) => {
      resolver.current = resolve;
    });
  }, []);
  const close = (value: boolean) => {
    resolver.current?.(value);
    resolver.current = null;
    setPending(null);
  };
  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      <ModalOverlay
        isOpen={pending !== null}
        onOpenChange={(open) => !open && close(false)}
        isDismissable
        className="ui-modal-overlay"
      >
        <Modal className="ui-modal">
          <Dialog role="alertdialog" className="ui-dialog">
            {pending && (
              <>
                <Heading slot="title" className="ui-dialog-title">
                  {pending.title}
                </Heading>
                <div className="ui-dialog-body">{pending.body}</div>
                <div className="ui-dialog-actions">
                  <Button autoFocus onPress={() => close(false)}>
                    {pending.cancelLabel ?? "Cancel"}
                  </Button>
                  <Button
                    variant={pending.destructive ? "danger" : "primary"}
                    onPress={() => close(true)}
                  >
                    {pending.confirmLabel}
                  </Button>
                </div>
              </>
            )}
          </Dialog>
        </Modal>
      </ModalOverlay>
    </ConfirmContext.Provider>
  );
}
export function useConfirm(): ConfirmFn {
  const confirm = useContext(ConfirmContext);
  if (!confirm) throw new Error("useConfirm must be used inside ConfirmProvider");
  return confirm;
}

// ---- Notices -------------------------------------------------------------------

export type NoticeTone = "info" | "success" | "warning" | "error";
/** Inline notice for page-level outcomes. Errors use role=alert, the rest role=status. */
export function Notice({
  tone = "info",
  children,
  action,
  onDismiss,
}: {
  tone?: NoticeTone;
  children: ReactNode;
  action?: ReactNode;
  onDismiss?: () => void;
}) {
  return (
    <div className={`ui-notice ${tone}`} role={tone === "error" ? "alert" : "status"}>
      <div className="ui-notice-body">{children}</div>
      {action}
      {onDismiss && (
        <AriaButton
          className="ui-button quiet ui-icon-button"
          aria-label="Dismiss notification"
          onPress={onDismiss}
        >
          <Icon name="x" />
        </AriaButton>
      )}
    </div>
  );
}

/** Sticky bar that appears only while there are unsaved changes (DESIGN.md save model). */
export function SaveBar({
  summary,
  detail,
  onSave,
  onCancel,
  isSaving,
}: {
  summary: ReactNode;
  detail: ReactNode;
  onSave: () => void;
  onCancel: () => void;
  isSaving?: boolean;
}) {
  return (
    <div className="savebar access-savebar" role="region" aria-label="Unsaved changes">
      <p>
        <strong>{summary}</strong> · <span>{detail}</span>
      </p>
      <div className="savebar-actions">
        <Button onPress={onCancel} isDisabled={isSaving}>
          Cancel
        </Button>
        <Button variant="primary" onPress={onSave} isPending={isSaving}>
          Save changes
        </Button>
      </div>
    </div>
  );
}

// ---- Icons (Lucide paths, decorative only; text labels always accompany them) -------------

const ICONS: Record<string, string> = {
  search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-4-4"/>',
  x: '<path d="M18 6 6 18M6 6l12 12"/>',
  more: '<circle cx="12" cy="5" r="1.4"/><circle cx="12" cy="12" r="1.4"/><circle cx="12" cy="19" r="1.4"/>',
  chevron: '<path d="m9 6 6 6-6 6"/>',
  check: '<path d="m5 12 4 4L19 6"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  shield: '<path d="M12 22s8-3.5 8-10V5l-8-3-8 3v7c0 6.5 8 10 8 10Z"/><path d="m9 12 2 2 4-5"/>',
  users:
    '<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.9M16 3.1a4 4 0 0 1 0 7.8"/>',
  plug: '<path d="M12 22v-5M9 8V2M15 8V2"/><path d="M18 8v5a6 6 0 0 1-12 0V8Z"/>',
  data: '<ellipse cx="12" cy="5" rx="8" ry="3"/><path d="M4 5v6c0 1.7 3.6 3 8 3s8-1.3 8-3V5"/><path d="M4 11v6c0 1.7 3.6 3 8 3s8-1.3 8-3v-6"/>',
  help: '<circle cx="12" cy="12" r="9"/><path d="M9.7 9a2.5 2.5 0 1 1 3.2 2.4c-.9.3-.9 1.1-.9 1.6"/><path d="M12 17h.01"/>',
  menu: '<path d="M4 6h16M4 12h16M4 18h16"/>',
  copy: '<rect x="9" y="9" width="12" height="12" rx="2"/><path d="M5 15H4a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1h10a1 1 0 0 1 1 1v1"/>',
  alert:
    '<path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0Z"/><path d="M12 9v4M12 17h.01"/>',
  circle: '<circle cx="12" cy="12" r="9"/>',
  arrow: '<path d="M5 12h14M13 6l6 6-6 6"/>',
  activity: '<path d="M22 12h-4l-3 9L9 3l-3 9H2"/>',
  file: '<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8Z"/><path d="M14 2v6h6"/>',
};
export function Icon({
  name,
  path,
  className = "",
}: {
  name?: string;
  path?: string;
  className?: string;
}) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.8}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
      className={`ui-icon ${className}`.trim()}
      dangerouslySetInnerHTML={{ __html: path ?? ICONS[name ?? ""] ?? "" }}
    />
  );
}
