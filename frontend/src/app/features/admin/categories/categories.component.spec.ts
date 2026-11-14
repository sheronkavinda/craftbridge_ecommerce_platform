import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of } from 'rxjs';
import { Category } from '../admin.models';
import { AdminService } from '../admin.service';
import { CategoriesComponent } from './categories.component';

describe('CategoriesComponent', () => {
  let component: CategoriesComponent;
  let fixture: ComponentFixture<CategoriesComponent>;
  let adminServiceSpy: jasmine.SpyObj<AdminService>;

  const mockCategories: Category[] = [
    {
      id: 'cat-001',
      name: 'Handwoven Textiles',
      slug: 'handwoven-textiles',
      description: 'Traditional handloom saris, scarves, and table runners.',
      is_active: true,
      created_at: '2026-11-01T10:00:00Z',
    },
    {
      id: 'cat-002',
      name: 'Wood Carving',
      slug: 'wood-carving',
      description: 'Artisanal masks and carved decor.',
      is_active: false,
      created_at: '2026-11-02T10:00:00Z',
    },
  ];

  beforeEach(async () => {
    adminServiceSpy = jasmine.createSpyObj<AdminService>('AdminService', [
      'listCategories',
      'createCategory',
      'updateCategory',
      'deleteCategory',
    ]);
    adminServiceSpy.listCategories.and.returnValue(of(mockCategories));
    adminServiceSpy.createCategory.and.returnValue(of(mockCategories[0]));
    adminServiceSpy.updateCategory.and.returnValue(of(mockCategories[0]));
    adminServiceSpy.deleteCategory.and.returnValue(of(void 0));

    await TestBed.configureTestingModule({
      imports: [CategoriesComponent],
      providers: [{ provide: AdminService, useValue: adminServiceSpy }],
    }).compileComponents();

    fixture = TestBed.createComponent(CategoriesComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('should render the category list from the mocked service', () => {
    const compiled = fixture.nativeElement as HTMLElement;
    const rows = compiled.querySelectorAll('tbody tr');
    expect(rows.length).toBe(2);
    expect(rows[0].textContent).toContain('Handwoven Textiles');
    expect(rows[0].textContent).toContain('handwoven-textiles');
    expect(rows[0].textContent).toContain('Active');
    expect(rows[1].textContent).toContain('Wood Carving');
    expect(rows[1].textContent).toContain('Inactive');
  });

  it('should display the empty state when no categories exist', () => {
    adminServiceSpy.listCategories.and.returnValue(of([]));
    component.loadCategories();
    fixture.detectChanges();

    const compiled = fixture.nativeElement as HTMLElement;
    expect(compiled.textContent).toContain('No categories yet');
  });

  it('should open the create dialog and auto-generate slug from name', () => {
    component.openCreateForm();
    fixture.detectChanges();

    const dialog = (fixture.nativeElement as HTMLElement).querySelector('[role="dialog"]');
    expect(dialog).toBeTruthy();

    component.categoryForm.controls.name.setValue('Brass & Copper Ware');
    expect(component.categoryForm.controls.slug.value).toBe('brass-copper-ware');
  });

  it('should open the delete confirmation dialog before calling deleteCategory', () => {
    component.requestDelete(mockCategories[0]);
    fixture.detectChanges();

    const dialog = (fixture.nativeElement as HTMLElement).querySelector('[role="dialog"]');
    expect(dialog?.textContent).toContain('Delete Handwoven Textiles?');
    expect(adminServiceSpy.deleteCategory).not.toHaveBeenCalled();

    component.confirmDelete();
    expect(adminServiceSpy.deleteCategory).toHaveBeenCalledWith('cat-001');
  });
});
